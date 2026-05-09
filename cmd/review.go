package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/config"
	"github.com/arinorr/prism/internal/diff"
	"github.com/arinorr/prism/internal/gh"
	gitpkg "github.com/arinorr/prism/internal/git"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/claude"
	"github.com/arinorr/prism/internal/report"
	"github.com/arinorr/prism/internal/sizecheck"
)

const (
	defaultResultsDir   = "results"
	defaultFilenameSlug = "unknown"
)

// prClient abstracts the GitHub client for testability.
type prClient interface {
	GetPRDiff(prRef string) (*gh.PR, error)
	PostComments(pr *gh.PR, suggestions []gh.Suggestion) error
}

// newGHClient creates a new GitHub client. Replaced in tests.
var newGHClient = func() (prClient, error) {
	return gh.NewClient()
}

// boolFlag tracks whether a boolean flag was explicitly set on the CLI.
// This distinguishes "not passed" (set=false) from "--no-verify" (set=true, value=false),
// so unset flags don't overwrite tier or config defaults.
type boolFlag struct {
	value bool
	set   bool
}

// reviewOptions holds parsed CLI flags for the review command.
type reviewOptions struct {
	prRef              string
	comment            bool
	verbose            bool
	dryRun             bool
	estimate           bool
	debate             bool
	toStdout           bool
	yes                bool
	rolesFlag          string
	formatFlag         string
	modelFlag          string
	tierFlag           string
	timeoutFlag        string
	retriesFlag        int
	budgetFlag         float64
	verifierBudgetFlag float64
	crossRefs          boolFlag
	verify             boolFlag
	noCompress         bool
	configPath         string
}

// parseStringFlag checks whether args[*i] matches --flag or --flag=value.
// For --flag value it advances *i and returns (value, true).
// For --flag=value it returns (value, true) without advancing.
// Otherwise it returns ("", false).
func parseStringFlag(args []string, i *int, flag string) (string, bool) {
	if args[*i] == flag && *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	if strings.HasPrefix(args[*i], flag+"=") {
		return strings.TrimPrefix(args[*i], flag+"="), true
	}
	return "", false
}

// parseReviewArgs parses the CLI arguments for the review command.
func parseReviewArgs(args []string) (*reviewOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	opts := &reviewOptions{configPath: ".prism.yml", retriesFlag: -1}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--comment":
			opts.comment = true
		case "--verbose", "-v":
			opts.verbose = true
		case "--dry-run":
			opts.dryRun = true
		case "--estimate":
			opts.estimate = true
		case "--debate":
			opts.debate = true
		case "--yes", "-y":
			opts.yes = true
		case "--verify":
			opts.verify = boolFlag{value: true, set: true}
		case "--no-verify":
			opts.verify = boolFlag{value: false, set: true}
		case "--cross-refs":
			opts.crossRefs = boolFlag{value: true, set: true}
		case "--no-cross-refs":
			opts.crossRefs = boolFlag{value: false, set: true}
		case "--no-compress":
			opts.noCompress = true
		case "--stdout":
			opts.toStdout = true
		default:
			if v, ok := parseStringFlag(args, &i, "--tier"); ok {
				opts.tierFlag = v
			} else if v, ok := parseStringFlag(args, &i, "--roles"); ok {
				opts.rolesFlag = v
			} else if v, ok := parseStringFlag(args, &i, "--format"); ok {
				opts.formatFlag = v
			} else if v, ok := parseStringFlag(args, &i, "--model"); ok {
				opts.modelFlag = v
			} else if v, ok := parseStringFlag(args, &i, "--timeout"); ok {
				opts.timeoutFlag = v
			} else if v, ok := parseStringFlag(args, &i, "--max-retries"); ok {
				var n int
				if _, scanErr := fmt.Sscanf(v, "%d", &n); scanErr == nil {
					opts.retriesFlag = n
				}
			} else if v, ok := parseStringFlag(args, &i, "--max-budget-usd"); ok {
				var f float64
				if _, scanErr := fmt.Sscanf(v, "%f", &f); scanErr == nil {
					opts.budgetFlag = f
				}
			} else if v, ok := parseStringFlag(args, &i, "--verifier-budget"); ok {
				var f float64
				if _, scanErr := fmt.Sscanf(v, "%f", &f); scanErr == nil {
					opts.verifierBudgetFlag = f
				}
			} else if v, ok := parseStringFlag(args, &i, "--config"); ok {
				opts.configPath = v
			} else if !strings.HasPrefix(args[i], "-") {
				opts.prRef = args[i]
			} else {
				return nil, fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	if opts.prRef == "" {
		return nil, fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	return opts, nil
}

// loadAndMergeConfig loads configuration from file and merges with CLI flags.
func loadAndMergeConfig(opts *reviewOptions) (config.Config, error) {
	fileCfg, cfgErr := config.Load(opts.configPath)
	if cfgErr != nil {
		return config.Config{}, fmt.Errorf("failed to load config: %w", cfgErr)
	}
	cliCfg := config.Config{
		Tier:              opts.tierFlag,
		Model:             opts.modelFlag,
		Format:            opts.formatFlag,
		AgentTimeout:      opts.timeoutFlag,
		MaxBudgetUSD:      opts.budgetFlag,
		VerifierBudgetUSD: opts.verifierBudgetFlag,
	}
	if opts.retriesFlag >= 0 {
		cliCfg.MaxRetries = config.IntPtr(opts.retriesFlag)
	}
	if opts.rolesFlag != "" {
		cliCfg.Roles = strings.Split(opts.rolesFlag, ",")
		for i := range cliCfg.Roles {
			cliCfg.Roles[i] = strings.TrimSpace(cliCfg.Roles[i])
		}
	}
	def := config.Default()
	return config.Merge(&def, &fileCfg, &cliCfg), nil
}

// Format names accepted by --format, both as user input and as ext suffixes
// on rendered files. Centralized so `goconst` doesn't trip on repetition.
const (
	formatPlain    = "plain"
	formatMD       = "md"
	formatMarkdown = "markdown"
	formatHTML     = "html"
	formatJSON     = "json"
	formatTxt      = "txt" // ext used for plain output written to file
)

// resolveTier applies the three-step last-write-wins resolution:
// 1. Determine tier (CLI --tier > config tier > default deep).
// 2. Expand tier into ReviewConfig defaults.
// 3. Apply config file field overrides, then CLI flag overrides.
func resolveTier(merged *config.Config, opts *reviewOptions) (agents.ReviewConfig, error) {
	// Step 1: pick tier — CLI wins over config wins over default.
	tier := agents.TierDeep
	if merged.Tier != "" {
		tier = agents.Tier(merged.Tier)
	}
	if opts.tierFlag != "" {
		tier = agents.Tier(opts.tierFlag)
	}
	if !tier.Valid() {
		return agents.ReviewConfig{}, fmt.Errorf("unknown tier %q (available: quick, standard, deep, thorough)", tier)
	}

	// Step 2: expand tier into defaults.
	rc := tier.Defaults()

	// Step 3: apply overrides. Model is already resolved by config.Merge
	// (CLI > config > default), so we apply it directly. CrossRefs and
	// Verify use pointer bools / boolFlags so unset values don't clobber
	// the tier defaults.
	if merged.Model != "" {
		rc.Model = merged.Model
	}
	// ScopeHints intentionally tracks CrossRefs — they are conceptually one
	// "codebase context" toggle and are exposed as a single --cross-refs CLI
	// flag (see tier.go ReviewConfig.ScopeHints comment).
	if merged.CrossRefs != nil {
		rc.CrossRefs = *merged.CrossRefs
		rc.ScopeHints = *merged.CrossRefs
	}
	if merged.Verify != nil {
		rc.Verify = *merged.Verify
	}
	// CLI boolFlags override config (last write wins).
	if opts.crossRefs.set {
		rc.CrossRefs = opts.crossRefs.value
		rc.ScopeHints = opts.crossRefs.value
	}
	if opts.verify.set {
		rc.Verify = opts.verify.value
	}

	return rc, nil
}

// validFormats lists the accepted values for --format.
var validFormats = map[string]bool{
	"":             true,
	formatPlain:    true,
	formatMD:       true,
	formatMarkdown: true,
	formatHTML:     true,
	formatJSON:     true,
}

// parseFormats splits a --format value on commas, trims whitespace from each
// entry, and returns non-empty results. "md, html " → ["md","html"]. "" → nil.
// Empty entries from "md,,html" are dropped silently (forgiving UX).
func parseFormats(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validateOptions performs fast, cheap validation of CLI flags so we can fail
// before spending tokens on LLM calls or network requests.
func validateOptions(opts *reviewOptions, merged *config.Config) error {
	// Resolve effective format: CLI flag > merged config.
	format := opts.formatFlag
	if format == "" {
		format = merged.Format
	}
	formats := parseFormats(format)
	for _, f := range formats {
		if !validFormats[f] {
			return fmt.Errorf("invalid format: %q (available: plain, md, html, json)", f)
		}
	}
	// --stdout is a single-destination output; reject combos here so we fail
	// before any LLM dispatch rather than after the orchestrator runs.
	if opts.toStdout && len(formats) > 1 {
		return fmt.Errorf("--stdout requires a single format, got %d (%s)", len(formats), strings.Join(formats, ","))
	}

	// Validate CLI timeout parses as a Go duration.
	if opts.timeoutFlag != "" {
		if _, err := time.ParseDuration(opts.timeoutFlag); err != nil {
			return fmt.Errorf("invalid timeout: %q (must be a Go duration like 30s, 2m, 1h)", opts.timeoutFlag)
		}
	}

	// Validate --tier early so a typo fails before the PR fetch.
	// resolveTier still validates merged.Tier (file config) at runtime.
	if opts.tierFlag != "" && !agents.Tier(opts.tierFlag).Valid() {
		return fmt.Errorf("unknown tier %q (available: quick, standard, deep, thorough)", opts.tierFlag)
	}

	return nil
}

// resolveRoles determines which agent roles to use from the merged config.
func resolveRoles(merged *config.Config) ([]agents.Role, error) {
	if len(merged.Roles) > 0 {
		return agents.ParseRoles(strings.Join(merged.Roles, ","))
	}
	return agents.AllRoles, nil
}

// fetchPR fetches the PR diff from GitHub.
func fetchPR(opts *reviewOptions) (*gh.PR, prClient, error) {
	client, err := newGHClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize GitHub client: %w", err)
	}

	pr, err := client.GetPRDiff(opts.prRef)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get PR diff: %w", err)
	}

	return pr, client, nil
}

// progress writes a message to stderr. All progress/diagnostic output
// goes to stderr so stdout is reserved for the report (Unix convention).
func progress(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// progressln writes a line to stderr.
func progressln(args ...any) {
	fmt.Fprintln(os.Stderr, args...)
}

// claudeCLIFallbackPaths lists install locations to probe when "claude"
// isn't on $PATH. Claude Code's installer puts the binary at
// ~/.claude/local/claude on macOS/Linux by default.
func claudeCLIFallbackPaths() []string {
	paths := []string{"/usr/local/bin/claude"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{filepath.Join(home, ".claude", "local", "claude")}, paths...)
	}
	return paths
}

// resolveClaudeCLI returns a path to the claude binary that exec.Command can
// invoke. Tries $PATH first, then known install locations. Returns an
// actionable error listing every path checked when nothing is found.
func resolveClaudeCLI() (string, error) {
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}
	fallbacks := claudeCLIFallbackPaths()
	for _, p := range fallbacks {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("claude CLI not found.\n"+
		"Checked: $PATH, %s\n"+
		"Install Claude Code: https://claude.ai/download\n"+
		"Or add an existing install to $PATH.",
		strings.Join(fallbacks, ", "))
}

func runReview(args []string, skillsFS fs.FS) error {
	// Pre-flight: resolve the claude CLI path. Probe $PATH first, then common
	// install locations (Claude Code installs to ~/.claude/local/ by default,
	// which isn't always on $PATH).
	claudePath, err := resolveClaudeCLI()
	if err != nil {
		return err
	}

	opts, err := parseReviewArgs(args)
	if err != nil {
		return err
	}

	merged, err := loadAndMergeConfig(opts)
	if err != nil {
		return err
	}

	if err := validateOptions(opts, &merged); err != nil {
		return err
	}

	roles, err := resolveRoles(&merged)
	if err != nil {
		return err
	}

	pr, client, err := fetchPR(opts)
	if err != nil {
		return err
	}

	// Detect languages for skill module loading.
	languages := agents.DetectLanguages(pr.Files)

	// Default progress: summary only.
	progress("🔍 Reviewing PR #%s: %s (%d files)\n", pr.Number, pr.Title, len(pr.Files))

	// Verbose: show languages and detailed info.
	if opts.verbose {
		if len(languages) > 0 {
			progress("   Languages: %s\n", strings.Join(languages, ", "))
		}
	}

	// Compress diff before passing to orchestrator — the orchestrator
	// doesn't need to know about compression, just receives a clean diff.
	diffOpts := diff.DefaultOptions()
	if opts.noCompress || merged.NoCompress {
		diffOpts = diff.NoCompression()
	} else {
		diffOpts.ContextLines = merged.DiffContextLinesVal()
		diffOpts.ExtraPatterns = merged.StripPatterns
	}
	compressed, compSummary := diff.Compress(pr.Diff, diffOpts)
	if opts.verbose && compSummary.OriginalBytes > 0 {
		savings := 100 - (compSummary.CompressedBytes*100)/compSummary.OriginalBytes
		progress("   📦 Diff compressed: %dKB → %dKB (-%d%%, %d files stripped)\n",
			compSummary.OriginalBytes/1024, compSummary.CompressedBytes/1024,
			savings, len(compSummary.FilesRemoved))
	}
	compressedPR := *pr
	compressedPR.Diff = compressed

	// Show detailed estimate in verbose/estimate mode.
	if opts.verbose || opts.estimate {
		printEstimate(len(compressed), roles)
	}
	if opts.estimate {
		return nil
	}

	// Resolve tier presets → config overrides → CLI flags into the final
	// ReviewConfig (rc). Done before the breakdown so the user sees the
	// actual model + verify state that will run.
	rc, err := resolveTier(&merged, opts)
	if err != nil {
		return err
	}

	// Brief breakdown before the prompt so the user knows what they're paying
	// for. Verbose mode already printed the full printEstimate above; --estimate
	// returned earlier, so this only fires on the default path.
	if !opts.verbose {
		printConfirmBreakdown(len(compressed), roles, rc.Model, rc.Verify)
	}

	// In non-interactive mode (CI), fail hard on very large diffs
	// to avoid burning tokens on reviews that won't be effective.
	sizeResult := sizecheck.Check(len(compressed), merged.DiffWarnBytes, merged.DiffChunkBytes)
	if sizeResult.SuggestChunk && !opts.yes && !isInteractive() {
		return fmt.Errorf("diff too large for effective review (%dKB). Use --yes to override or split the PR",
			len(compressed)/1024)
	}

	if !opts.yes && !opts.dryRun && isInteractive() {
		fmt.Fprint(os.Stderr, "Continue? [Y/n] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "n" || answer == "no" {
			return fmt.Errorf("review canceled")
		}
	}

	// Resolve repo root for symbol indexing. Handles cross-repo PRs by
	// shallow-cloning the remote repo if needed.
	repoRoot, repoCleanup, repoErr := gitpkg.ResolveRepoForPR(pr.OwnerRepo, pr.HeadRef)
	if repoCleanup != nil {
		defer repoCleanup()
		progress("📥 Cloned %s for code analysis\n", pr.OwnerRepo)
	}
	if repoErr != nil {
		progress("   ⚠️  Repo resolution: %v (using local)\n", repoErr)
	}

	// Dispatch agents. Orchestrator progress goes to stderr.
	llmBackend := claude.New(claudePath)
	orchestrator, orchErr := agents.NewOrchestrator(roles, &agents.Options{
		Verbose:           opts.verbose,
		DryRun:            opts.dryRun,
		Debate:            opts.debate || merged.Debate,
		Model:             rc.Model,
		AgentTimeout:      merged.TimeoutDuration(),
		MaxRetries:        merged.MaxRetriesVal(),
		MaxBudgetUSD:      merged.MaxBudgetUSD,
		Verify:            rc.Verify,
		VerifierModel:     merged.VerifierModel,
		VerifierBudgetUSD: merged.VerifierBudgetUSD,
		RepoRoot:          repoRoot,
		Languages:         languages,
		ExplicitRoles:     opts.rolesFlag != "",
		CrossRefs:         rc.CrossRefs,
		ScopeHints:        rc.ScopeHints,
		SkillsFS:          skillsFS,
		Out:               os.Stderr,
		ErrOut:            os.Stderr,
	}, llmBackend, languages)
	if orchErr != nil {
		return fmt.Errorf("failed to initialize orchestrator: %w", orchErr)
	}

	ctx := context.Background()
	start := time.Now()
	result, err := orchestrator.Review(ctx, &compressedPR)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}
	elapsed := time.Since(start)

	// Default progress: one-line summary.
	u := result.Usage
	totalInput := u.TotalInputTokens()
	findingCount := len(result.DedupedFindings)
	if findingCount == 0 {
		findingCount = len(result.Findings)
	}
	progress("✅ %d findings | $%.2f | %s\n", findingCount, u.CostUSD, elapsed.Round(time.Second))

	// Verbose: detailed token breakdown.
	if opts.verbose {
		totalTokens := totalInput + u.OutputTokens
		progress("   📊 Tokens: %dk input, %dk output (%dk total)\n",
			totalInput/1000, u.OutputTokens/1000, totalTokens/1000)

		if len(result.AgentUsages) > 0 {
			for _, au := range result.AgentUsages {
				agentTotal := au.Usage.TotalTokens()
				progress("      %-16s %6dk tokens  $%.2f\n", au.Role, agentTotal/1000, au.Usage.CostUSD)
			}
		}
	}

	if result.DismissedCount > 0 || result.DowngradedCount > 0 {
		progress("   🔬 Verifier: %d dismissed, %d downgraded | Cost: $%.2f\n",
			result.DismissedCount, result.DowngradedCount, result.VerifierUsage.CostUSD)
	}
	if result.VerifierError != "" {
		progress("   ⚠️  Verifier error: %s\n", result.VerifierError)
	}

	// Resolve format: CLI flag > config file > none.
	formatFlag := opts.formatFlag
	if formatFlag == "" {
		formatFlag = merged.Format
	}

	// Output the results. This is the ONLY thing that goes to stdout
	// (when --stdout is set) or to a file.
	if err := outputResults(opts, pr, result, roles, elapsed, formatFlag); err != nil {
		return err
	}

	// Optionally post comments.
	if opts.comment {
		if len(result.Suggestions) == 0 {
			progress("\nNo inline suggestions to post.\n")
		} else {
			if err := client.PostComments(pr, result.Suggestions); err != nil {
				return fmt.Errorf("failed to post comments: %w", err)
			}
			progress("\n✅ Posted %d inline comments to PR #%s\n", len(result.Suggestions), pr.Number)
		}
	}

	return nil
}

// outputPath returns the on-disk path for a report. Filenames use vault-style
// YYMMDD-HHMMSS prefix so reports sort chronologically alongside notes/plans
// and reruns within the same minute don't overwrite each other.
//
// ownerRepo is "owner/repo" when the gh-CLI path is available, falling back
// to the bare repo name in the git-only path. The slash is mapped to "-"
// before sanitizeFilename runs, since sanitizeFilename otherwise strips
// everything before the last "/" (it's also used for URL-style PR refs).
func outputPath(ownerRepo, prNum, ext string, now time.Time) string {
	repo := sanitizeFilename(strings.ReplaceAll(ownerRepo, "/", "-"))
	prNum = sanitizeFilename(prNum)
	timestamp := now.Format("060102-150405")
	dir := filepath.Join(defaultResultsDir, fmt.Sprintf("%s-pr-%s", repo, prNum))
	return filepath.Join(dir, fmt.Sprintf("%s-%s-pr-%s.%s", timestamp, repo, prNum, ext))
}

// prRepoIdent returns the best identifier for a PR's repo for filename use:
// "owner/repo" if available (gh-CLI path), else the bare repo name (git-only
// fallback path, where pr.OwnerRepo is empty).
func prRepoIdent(pr *gh.PR) string {
	if pr.OwnerRepo != "" {
		return pr.OwnerRepo
	}
	return pr.Repo
}

// renderFormat builds the rendered report for a single format. Each format
// pulls from the same in-memory Data, so generating multiple formats is free
// (no extra LLM calls — just additional render passes).
func renderFormat(format string, data *report.Data, summary string) (output, ext string, err error) {
	switch format {
	case formatPlain:
		return summary + "\n", formatTxt, nil
	case formatMD, formatMarkdown:
		return report.Markdown(data), formatMD, nil
	case formatHTML:
		out, err := report.HTML(data)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate HTML report: %w", err)
		}
		return out, formatHTML, nil
	case formatJSON:
		out, err := report.JSON(data)
		if err != nil {
			return "", "", fmt.Errorf("failed to generate JSON report: %w", err)
		}
		return out, formatJSON, nil
	default:
		return "", "", fmt.Errorf("unknown format: %s (available: md, html, json, plain)", format)
	}
}

// outputResults handles format selection, report generation, and file output.
// When formatFlag contains comma-separated formats (e.g. "md,html"), one file
// is written per format from the same in-memory Data — no additional API cost.
func outputResults(opts *reviewOptions, pr *gh.PR, result *agents.ReviewResult, roles []agents.Role, elapsed time.Duration, formatFlag string) error {
	formats := parseFormats(formatFlag)
	if len(formats) == 0 {
		fmt.Println(result.Summary)
		return nil
	}

	roleNames := make([]string, len(roles))
	for i := range roles {
		roleNames[i] = roles[i].Name
	}
	data := &report.Data{
		PR:              pr,
		Result:          result,
		Roles:           roleNames,
		Duration:        elapsed.Round(time.Second).String(),
		Usage:           result.Usage,
		DismissedCount:  result.DismissedCount,
		DowngradedCount: result.DowngradedCount,
		VerifierError:   result.VerifierError,
	}

	// Single time.Now() so all formats from one review share the same
	// YYMMDD-HHMM filename prefix.
	now := time.Now()
	for _, f := range formats {
		output, ext, err := renderFormat(f, data, result.Summary)
		if err != nil {
			return err
		}
		if opts.toStdout {
			fmt.Print(output)
			continue
		}
		if err := writeToFile(output, outputPath(prRepoIdent(pr), pr.Number, ext, now)); err != nil {
			return err
		}
	}
	return nil
}

// Estimation constants calibrated against Claude Sonnet (April 2026).
const (
	// bytesPerToken is the average bytes per token for code (UTF-8).
	bytesPerToken = 4
	// promptOverheadTokens is the approximate overhead per agent from the
	// skill prompt, PR metadata, and output format instructions.
	promptOverheadTokens = 2000
	// expectedOutputTokens is the average output per agent (findings JSON).
	expectedOutputTokens = 1000
)

// capitalize uppercases the first byte if it's ASCII [a-z], else returns s
// unchanged. ASCII-only — do not use for arbitrary user input. Intended for
// fixed model names like haiku/sonnet/opus → Haiku/Sonnet/Opus.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// estimateCost returns the projected USD cost for a review.
func estimateCost(diffBytes int, roles []agents.Role, modelOverride string) float64 {
	tokensPerAgent := diffBytes/bytesPerToken + promptOverheadTokens
	modelCounts := make(map[string]int)
	for i := range roles {
		m := modelOverride
		if m == "" {
			m = roles[i].Model
		}
		if m == "" {
			m = agents.ModelTierStandard
		}
		modelCounts[m]++
	}
	var cost float64
	for model, count := range modelCounts {
		pricing := llm.ModelPricing(model)
		cost += pricing.EstimateCost(tokensPerAgent*count, expectedOutputTokens*count)
	}
	return cost
}

// modelSummary describes the agent model assignment in one short phrase
// suitable for the pre-prompt breakdown line.
func modelSummary(roles []agents.Role, modelOverride string) string {
	if modelOverride != "" {
		return "all " + capitalize(modelOverride)
	}
	seen := map[string]bool{}
	for i := range roles {
		m := roles[i].Model
		if m == "" {
			m = agents.ModelTierStandard
		}
		seen[m] = true
	}
	if len(seen) == 1 {
		for m := range seen {
			return "all " + capitalize(m)
		}
	}
	// Multiple models — show in a stable order.
	var parts []string
	for _, m := range []string{agents.ModelTierFast, agents.ModelTierStandard, agents.ModelTierDeep} {
		if seen[m] {
			parts = append(parts, capitalize(m))
		}
	}
	return "per-role (" + strings.Join(parts, "/") + ")"
}

// printConfirmBreakdown prints a short pre-prompt summary so the user knows
// what they're about to spend tokens on (which agents/models, whether the
// verifier runs, and the dollar estimate).
func printConfirmBreakdown(diffBytes int, roles []agents.Role, modelOverride string, verify bool) {
	verifyState := "off"
	if verify {
		verifyState = "on"
	}
	progress("   Models: %s | Verify: %s\n", modelSummary(roles, modelOverride), verifyState)
	progress("   💰 Estimated cost: ~$%.2f (%d agents)\n", estimateCost(diffBytes, roles, modelOverride), len(roles))
}

// printEstimate shows projected token usage based on diff size and roles.
func printEstimate(diffBytes int, roles []agents.Role) {
	tokensPerAgent := diffBytes/bytesPerToken + promptOverheadTokens
	totalInput := tokensPerAgent * len(roles)
	totalOutput := expectedOutputTokens * len(roles)
	total := totalInput + totalOutput

	// Group roles by model for cost breakdown.
	modelCounts := make(map[string]int)
	for i := range roles {
		model := roles[i].Model
		if model == "" {
			model = agents.ModelTierStandard
		}
		modelCounts[model]++
	}

	// Estimate cost per model tier using pricing from the LLM layer.
	var costEstimate float64
	for model, count := range modelCounts {
		pricing := llm.ModelPricing(model)
		costEstimate += pricing.EstimateCost(tokensPerAgent*count, expectedOutputTokens*count)
	}

	progressln("📏 Token estimate (approximate):")
	progress("   Diff size:    %d bytes (~%dk tokens per agent)\n", diffBytes, tokensPerAgent/1000)
	progress("   Agents:       %d\n", len(roles))

	// Show model breakdown so users understand cost drivers.
	for _, model := range []string{agents.ModelTierDeep, agents.ModelTierStandard, agents.ModelTierFast} {
		count := modelCounts[model]
		if count == 0 {
			continue
		}
		var names []string
		for i := range roles {
			m := roles[i].Model
			if m == "" {
				m = agents.ModelTierStandard
			}
			if m == model {
				names = append(names, roles[i].Name)
			}
		}
		progress("     %-6s ×%d    %s\n", model, count, strings.Join(names, ", "))
	}

	progress("   Est. input:   ~%dk tokens\n", totalInput/1000)
	progress("   Est. output:  ~%dk tokens\n", totalOutput/1000)
	progress("   Est. total:   ~%dk tokens\n", total/1000)
	progress("   Est. cost:    ~$%.2f\n", costEstimate)
	progress("   Verifier:     ~$%.2f (20%% of agent cost, configurable via --verifier-budget)\n", costEstimate*0.20)
	progressln("\n   Note: actual cost varies by caching and response length.")
}

var filenameAllowlist = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// sanitizeFilename strips all characters except alphanumeric, underscore, and hyphen.
func sanitizeFilename(s string) string {
	// Extract just the last path segment if it's a URL-style ref.
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		s = s[idx+1:]
	}
	s = filenameAllowlist.ReplaceAllString(s, "")
	if s == "" {
		return defaultFilenameSlug
	}
	return s
}

// isInteractive returns true if stdin is a terminal (not piped or in CI).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// writeToFile writes content to the given path, creating directories as needed.
// The path is constructed from defaultResultsDir + sanitizeFilename(prNumber) + extension,
// where sanitizeFilename strips all characters except [a-zA-Z0-9_-], preventing
// path traversal via crafted PR numbers.
func writeToFile(content, path string) error {
	dir := filepath.Dir(filepath.Clean(path))
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- path is built from sanitizeFilename which strips traversal chars
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(filepath.Clean(path), []byte(content), 0o600); err != nil { // #nosec G703 -- same as above
		return fmt.Errorf("failed to write output to %s: %w", path, err)
	}
	progress("📄 Report written to %s\n", path)
	return nil
}

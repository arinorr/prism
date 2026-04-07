package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/config"
	"github.com/arinorr/prism/internal/diff"
	"github.com/arinorr/prism/internal/gh"
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

// reviewOptions holds parsed CLI flags for the review command.
type reviewOptions struct {
	prRef       string
	comment     bool
	verbose     bool
	dryRun      bool
	estimate    bool
	debate      bool
	toStdout    bool
	yes         bool
	rolesFlag   string
	formatFlag  string
	modelFlag   string
	timeoutFlag string
	retriesFlag int
	budgetFlag  float64
	noCompress  bool
	configPath  string
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
		case "--no-compress":
			opts.noCompress = true
		case "--stdout":
			opts.toStdout = true
		default:
			if v, ok := parseStringFlag(args, &i, "--roles"); ok {
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
		Model:        opts.modelFlag,
		Format:       opts.formatFlag,
		AgentTimeout: opts.timeoutFlag,
		MaxBudgetUSD: opts.budgetFlag,
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

// validFormats lists the accepted values for --format.
var validFormats = map[string]bool{
	"":         true,
	"plain":    true,
	"md":       true,
	"markdown": true,
	"html":     true,
	"json":     true,
}

// validateOptions performs fast, cheap validation of CLI flags so we can fail
// before spending tokens on LLM calls or network requests.
func validateOptions(opts *reviewOptions, merged *config.Config) error {
	// Resolve effective format: CLI flag > merged config.
	format := opts.formatFlag
	if format == "" {
		format = merged.Format
	}
	if !validFormats[format] {
		return fmt.Errorf("invalid format: %q (available: plain, md, html, json)", format)
	}

	// Validate CLI timeout parses as a Go duration.
	if opts.timeoutFlag != "" {
		if _, err := time.ParseDuration(opts.timeoutFlag); err != nil {
			return fmt.Errorf("invalid timeout: %q (must be a Go duration like 30s, 2m, 1h)", opts.timeoutFlag)
		}
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

func runReview(args []string) error {
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

	// Route agents based on PR content (unless --roles was explicitly set).
	if opts.rolesFlag == "" {
		paths := make([]string, len(pr.Files))
		for i, f := range pr.Files {
			paths[i] = f.Path
		}
		category := agents.ClassifyPR(paths)
		routed := agents.RouteAgents(roles, category)
		if len(routed) < len(roles) {
			names := make([]string, len(routed))
			for i, r := range routed {
				names[i] = r.Name
			}
			fmt.Printf("   📋 %s PR detected — running %d/%d agents: %s (use --roles to override)\n",
				category, len(routed), len(roles), strings.Join(names, ", "))
			if opts.verbose {
				for _, p := range paths {
					fmt.Printf("      %s → %s\n", p, agents.ClassifyFile(p))
				}
			}
		}
		roles = routed
	}

	// Detect languages for skill module loading.
	languages := agents.DetectLanguages(pr.Files)

	fmt.Printf("🔍 Reviewing PR #%s: %s\n", pr.Number, pr.Title)
	fmt.Printf("   %d files changed\n", len(pr.Files))
	if len(languages) > 0 {
		fmt.Printf("   Languages: %s\n", strings.Join(languages, ", "))
	}
	fmt.Println()

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
		fmt.Printf("   📦 Diff compressed: %dKB → %dKB (-%d%%, %d files stripped)\n",
			compSummary.OriginalBytes/1024, compSummary.CompressedBytes/1024,
			savings, len(compSummary.FilesRemoved))
	}
	compressedPR := *pr
	compressedPR.Diff = compressed

	// Show estimate. In --estimate mode, print and exit.
	printEstimate(len(compressed), roles)
	if opts.estimate {
		return nil
	}

	// In non-interactive mode (CI), fail hard on very large diffs
	// to avoid burning tokens on reviews that won't be effective.
	sizeResult := sizecheck.Check(len(compressed), merged.DiffWarnBytes, merged.DiffChunkBytes)
	if sizeResult.SuggestChunk && !opts.yes && !isInteractive() {
		return fmt.Errorf("diff too large for effective review (%dKB). Use --yes to override or split the PR",
			len(compressed)/1024)
	}

	if !opts.yes && !opts.dryRun && isInteractive() {
		fmt.Print("Continue? [Y/n] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer == "n" || answer == "no" {
			return fmt.Errorf("review canceled")
		}
	}
	fmt.Println()

	// Dispatch agents.
	llmBackend := claude.New()
	orchestrator, orchErr := agents.NewOrchestrator(roles, &agents.Options{
		Verbose:      opts.verbose,
		DryRun:       opts.dryRun,
		Debate:       opts.debate || merged.Debate,
		Model:        merged.Model,
		AgentTimeout: merged.TimeoutDuration(),
		MaxRetries:   merged.MaxRetriesVal(),
		MaxBudgetUSD: merged.MaxBudgetUSD,
	}, llmBackend, languages)
	if orchErr != nil {
		return fmt.Errorf("failed to initialize orchestrator: %w", orchErr)
	}

	start := time.Now()
	result, err := orchestrator.Review(&compressedPR)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}
	elapsed := time.Since(start)

	// Print usage summary. Input tokens include cache hits/misses since the
	// Claude CLI reports cached tokens separately from uncached ones.
	u := result.Usage
	totalInput := u.TotalInputTokens()
	totalTokens := totalInput + u.OutputTokens
	fmt.Printf("   📊 Tokens: %dk input, %dk output (%dk total) | Cost: $%.2f | Time: %s\n",
		totalInput/1000, u.OutputTokens/1000, totalTokens/1000, u.CostUSD, elapsed.Round(time.Second))

	if opts.verbose && len(result.AgentUsages) > 0 {
		fmt.Println()
		for _, au := range result.AgentUsages {
			agentTotal := au.Usage.TotalTokens()
			fmt.Printf("      %-16s %6dk tokens  $%.2f\n", au.Role, agentTotal/1000, au.Usage.CostUSD)
		}
	}
	fmt.Println()

	// Resolve format: CLI flag > config file > none.
	formatFlag := opts.formatFlag
	if formatFlag == "" {
		formatFlag = merged.Format
	}

	// Output the results.
	if err := outputResults(opts, pr, result, roles, elapsed, formatFlag); err != nil {
		return err
	}

	// Optionally post comments.
	if opts.comment {
		if len(result.Suggestions) == 0 {
			fmt.Println("\nNo inline suggestions to post.")
		} else {
			if err := client.PostComments(pr, result.Suggestions); err != nil {
				return fmt.Errorf("failed to post comments: %w", err)
			}
			fmt.Printf("\n✅ Posted %d inline comments to PR #%s\n", len(result.Suggestions), pr.Number)
		}
	}

	return nil
}

// outputResults handles format selection, report generation, and file output.
func outputResults(opts *reviewOptions, pr *gh.PR, result *agents.ReviewResult, roles []agents.Role, elapsed time.Duration, formatFlag string) error {
	if formatFlag == "" {
		fmt.Println(result.Summary)
		return nil
	}

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}
	data := &report.Data{
		PR:       pr,
		Result:   result,
		Roles:    roleNames,
		Duration: elapsed.Round(time.Second).String(),
		Usage:    result.Usage,
	}

	var output string
	ext := formatFlag
	if ext == "markdown" {
		ext = "md"
	}

	switch formatFlag {
	case "plain":
		fmt.Println(result.Summary)
		return nil
	case "md", "markdown":
		output = report.Markdown(data)
	case "html":
		var htmlErr error
		output, htmlErr = report.HTML(data)
		if htmlErr != nil {
			return fmt.Errorf("failed to generate HTML report: %w", htmlErr)
		}
	case "json":
		var jsonErr error
		output, jsonErr = report.JSON(data)
		if jsonErr != nil {
			return fmt.Errorf("failed to generate JSON report: %w", jsonErr)
		}
	default:
		return fmt.Errorf("unknown format: %s (available: md, html, json)", formatFlag)
	}

	if opts.toStdout {
		fmt.Print(output)
		return nil
	}

	repo := sanitizeFilename(pr.Repo)
	prNum := sanitizeFilename(pr.Number)
	timestamp := time.Now().Format("2006-01-02_3-04pm")
	dir := filepath.Join(defaultResultsDir, fmt.Sprintf("%s-pr-%s", repo, prNum))
	outPath := filepath.Join(dir, fmt.Sprintf("%s-pr-%s-%s.%s", repo, prNum, timestamp, ext))
	return writeToFile(output, outPath)
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

// printEstimate shows projected token usage based on diff size and roles.
func printEstimate(diffBytes int, roles []agents.Role) {
	tokensPerAgent := diffBytes/bytesPerToken + promptOverheadTokens
	totalInput := tokensPerAgent * len(roles)
	totalOutput := expectedOutputTokens * len(roles)
	total := totalInput + totalOutput

	// Group roles by model for cost breakdown.
	modelCounts := make(map[string]int)
	for _, r := range roles {
		model := r.Model
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

	fmt.Println("📏 Token estimate (approximate):")
	fmt.Printf("   Diff size:    %d bytes (~%dk tokens per agent)\n", diffBytes, tokensPerAgent/1000)
	fmt.Printf("   Agents:       %d\n", len(roles))

	// Show model breakdown so users understand cost drivers.
	for _, model := range []string{agents.ModelTierDeep, agents.ModelTierStandard, agents.ModelTierFast} {
		count := modelCounts[model]
		if count == 0 {
			continue
		}
		var names []string
		for _, r := range roles {
			m := r.Model
			if m == "" {
				m = agents.ModelTierStandard
			}
			if m == model {
				names = append(names, r.Name)
			}
		}
		fmt.Printf("     %-6s ×%d    %s\n", model, count, strings.Join(names, ", "))
	}

	fmt.Printf("   Est. input:   ~%dk tokens\n", totalInput/1000)
	fmt.Printf("   Est. output:  ~%dk tokens\n", totalOutput/1000)
	fmt.Printf("   Est. total:   ~%dk tokens\n", total/1000)
	fmt.Printf("   Est. cost:    ~$%.2f\n", costEstimate)
	fmt.Println("\n   Note: actual cost varies by caching and response length.")
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
	fmt.Printf("📄 Report written to %s\n", path)
	return nil
}

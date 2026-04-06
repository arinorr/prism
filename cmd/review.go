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

// reviewOptions holds parsed CLI flags for the review command.
type reviewOptions struct {
	prRef       string
	comment     bool
	verbose     bool
	dryRun      bool
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

// fetchAndCheckPR fetches the PR diff and checks its size, prompting for confirmation if large.
func fetchAndCheckPR(client prClient, opts *reviewOptions, merged *config.Config) (*gh.PR, error) {
	pr, err := client.GetPRDiff(opts.prRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR diff: %w", err)
	}

	// Check diff size and prompt for confirmation if large.
	sizeResult := sizecheck.Check(len(pr.Diff), merged.DiffWarnBytes, merged.DiffChunkBytes)
	if sizeResult.Warn {
		fmt.Fprintf(os.Stderr, "⚠️  %s\n", sizeResult.Message)
		if !opts.yes && isInteractive() {
			fmt.Fprint(os.Stderr, "Continue anyway? [y/N] ")
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
			if answer != "y" && answer != "yes" {
				return nil, fmt.Errorf("review canceled — diff too large")
			}
		}
	}

	return pr, nil
}

func runReview(args []string) error {
	client, err := gh.NewClient()
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub client: %w", err)
	}
	return runReviewWith(args, client)
}

func runReviewWith(args []string, client prClient) error {
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

	pr, err := fetchAndCheckPR(client, opts, &merged)
	if err != nil {
		return err
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

	// Dispatch agents.
	llmBackend := claude.New()
	orchestrator, orchErr := agents.NewOrchestrator(roles, &agents.Options{
		Verbose:      opts.verbose,
		DryRun:       opts.dryRun,
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

	if result.DryRun != nil {
		printDryRun(result.DryRun, opts.verbose)
		return nil
	}

	elapsed := time.Since(start)

	// Print usage summary. Input tokens include cache hits/misses since the
	// Claude CLI reports cached tokens separately from uncached ones.
	u := result.Usage
	totalInput := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	totalTokens := totalInput + u.OutputTokens
	fmt.Printf("   📊 Tokens: %dk input, %dk output (%dk total) | Cost: $%.2f | Time: %s\n\n",
		totalInput/1000, u.OutputTokens/1000, totalTokens/1000, u.CostUSD, elapsed.Round(time.Second))

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
	timestamp := time.Now().Format("20060102-150405")
	dir := filepath.Join(defaultResultsDir, fmt.Sprintf("%s-pr-%s", repo, prNum))
	outPath := filepath.Join(dir, fmt.Sprintf("%s-pr-%s-%s.%s", repo, prNum, timestamp, ext))
	return writeToFile(output, outPath)
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

const previewMaxBytes = 500

// printDryRun formats and prints the dry run result to stdout.
func printDryRun(dr *agents.DryRunResult, verbose bool) {
	fmt.Println("🏜️  DRY RUN — no agents will be called")
	fmt.Printf("Diff size: %d bytes\n\n", dr.DiffBytes)

	fmt.Printf("Agents that would run (%d):\n", len(dr.Roles))
	for _, r := range dr.Roles {
		fmt.Printf("   • %s — %s\n", r.Name, r.Description)
		if verbose {
			fmt.Printf("     Skill file: %s (%d bytes)\n", r.SkillFile, dr.SkillSizes[r.Slug])
		}
	}

	if verbose {
		if dr.Model != "" {
			fmt.Printf("\nModel: %s\n", dr.Model)
		}
		if dr.Timeout > 0 {
			fmt.Printf("Agent timeout: %s\n", dr.Timeout)
		}
		fmt.Printf("Max retries: %d\n", dr.MaxRetries)
	}

	if len(dr.Roles) == 0 {
		return
	}
	fmt.Printf("\nSample prompt (for %s):\n", dr.Roles[0].Name)
	fmt.Println("───────────────────────────────────────")
	if len(dr.SamplePrompt) > previewMaxBytes {
		fmt.Printf("%s\n... (%d bytes total)\n", dr.SamplePrompt[:previewMaxBytes], len(dr.SamplePrompt))
	} else {
		fmt.Println(dr.SamplePrompt)
	}
	fmt.Println("───────────────────────────────────────")
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

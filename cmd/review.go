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
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/report"
	"github.com/arinorr/prism/internal/sizecheck"
)

const defaultResultsDir = "results"

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
	configPath  string
}

// parseReviewArgs parses the CLI arguments for the review command.
func parseReviewArgs(args []string) (*reviewOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	opts := &reviewOptions{configPath: ".prism.yml"}

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--comment":
			opts.comment = true
		case args[i] == "--verbose" || args[i] == "-v":
			opts.verbose = true
		case args[i] == "--dry-run":
			opts.dryRun = true
		case args[i] == "--yes" || args[i] == "-y":
			opts.yes = true
		case args[i] == "--stdout":
			opts.toStdout = true
		case args[i] == "--roles" && i+1 < len(args):
			i++
			opts.rolesFlag = args[i]
		case strings.HasPrefix(args[i], "--roles="):
			opts.rolesFlag = strings.TrimPrefix(args[i], "--roles=")
		case args[i] == "--format" && i+1 < len(args):
			i++
			opts.formatFlag = args[i]
		case strings.HasPrefix(args[i], "--format="):
			opts.formatFlag = strings.TrimPrefix(args[i], "--format=")
		case args[i] == "--model" && i+1 < len(args):
			i++
			opts.modelFlag = args[i]
		case strings.HasPrefix(args[i], "--model="):
			opts.modelFlag = strings.TrimPrefix(args[i], "--model=")
		case args[i] == "--timeout" && i+1 < len(args):
			i++
			opts.timeoutFlag = args[i]
		case strings.HasPrefix(args[i], "--timeout="):
			opts.timeoutFlag = strings.TrimPrefix(args[i], "--timeout=")
		case args[i] == "--max-retries" && i+1 < len(args):
			i++
			var n int
			if _, scanErr := fmt.Sscanf(args[i], "%d", &n); scanErr == nil {
				opts.retriesFlag = n
			}
		case strings.HasPrefix(args[i], "--max-retries="):
			var n int
			if _, scanErr := fmt.Sscanf(strings.TrimPrefix(args[i], "--max-retries="), "%d", &n); scanErr == nil {
				opts.retriesFlag = n
			}
		case args[i] == "--config" && i+1 < len(args):
			i++
			opts.configPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			opts.configPath = strings.TrimPrefix(args[i], "--config=")
		case !strings.HasPrefix(args[i], "-"):
			opts.prRef = args[i]
		default:
			return nil, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if opts.prRef == "" {
		return nil, fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	return opts, nil
}

func runReview(args []string) error {
	opts, err := parseReviewArgs(args)
	if err != nil {
		return err
	}

	// Load and merge config: defaults < .prism.yml < CLI flags.
	fileCfg, cfgErr := config.Load(opts.configPath)
	if cfgErr != nil {
		return fmt.Errorf("failed to load config: %w", cfgErr)
	}
	cliCfg := config.Config{
		Model:        opts.modelFlag,
		Format:       opts.formatFlag,
		AgentTimeout: opts.timeoutFlag,
		MaxRetries:   opts.retriesFlag,
	}
	if opts.rolesFlag != "" {
		cliCfg.Roles = strings.Split(opts.rolesFlag, ",")
		for i := range cliCfg.Roles {
			cliCfg.Roles[i] = strings.TrimSpace(cliCfg.Roles[i])
		}
	}
	def := config.Default()
	merged := config.Merge(&def, &fileCfg, &cliCfg)

	// Determine which roles to use.
	roles := agents.AllRoles
	if len(merged.Roles) > 0 {
		var parseErr error
		roles, parseErr = agents.ParseRoles(strings.Join(merged.Roles, ","))
		if parseErr != nil {
			return parseErr
		}
	}

	// Fetch PR diff.
	client, err := gh.NewClient()
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub client: %w", err)
	}

	pr, err := client.GetPRDiff(opts.prRef)
	if err != nil {
		return fmt.Errorf("failed to get PR diff: %w", err)
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
				return fmt.Errorf("review canceled — diff too large")
			}
		}
	}

	// Detect languages for skill module loading.
	languages := agents.DetectLanguages(pr.Files)

	fmt.Printf("🔍 Reviewing PR #%s: %s\n", pr.Number, pr.Title)
	fmt.Printf("   %d files changed\n", len(pr.Files))
	if len(languages) > 0 {
		fmt.Printf("   Languages: %s\n", strings.Join(languages, ", "))
	}
	fmt.Println()

	// Dispatch agents.
	orchestrator, orchErr := agents.NewOrchestrator(roles, agents.Options{
		Verbose:      opts.verbose,
		DryRun:       opts.dryRun,
		Model:        merged.Model,
		AgentTimeout: merged.TimeoutDuration(),
		MaxRetries:   merged.MaxRetries,
	}, languages)
	if orchErr != nil {
		return fmt.Errorf("failed to initialize orchestrator: %w", orchErr)
	}

	start := time.Now()
	result, err := orchestrator.Review(pr)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}
	elapsed := time.Since(start)

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
	}

	var output string
	ext := formatFlag
	if ext == "markdown" {
		ext = "md"
	}

	switch formatFlag {
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

	outPath := filepath.Join(defaultResultsDir, fmt.Sprintf("prism-pr-%s.%s", sanitizeFilename(pr.Number), ext))
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
		return "unknown"
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

func writeToFile(content, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write output to %s: %w", path, err)
	}
	fmt.Printf("📄 Report written to %s\n", path)
	return nil
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm/claude"
	"github.com/arinorr/prism/internal/report"
)

const defaultResultsDir = "results"

// reviewOptions holds parsed CLI flags for the review command.
type reviewOptions struct {
	prRef      string
	comment    bool
	verbose    bool
	dryRun     bool
	toStdout   bool
	rolesFlag  string
	formatFlag string
}

// parseReviewArgs parses the CLI arguments for the review command.
func parseReviewArgs(args []string) (*reviewOptions, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	opts := &reviewOptions{}

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--comment":
			opts.comment = true
		case args[i] == "--verbose" || args[i] == "-v":
			opts.verbose = true
		case args[i] == "--dry-run":
			opts.dryRun = true
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

	// Determine which roles to use.
	roles := agents.AllRoles
	if opts.rolesFlag != "" {
		var parseErr error
		roles, parseErr = agents.ParseRoles(opts.rolesFlag)
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

	fmt.Printf("🔍 Reviewing PR #%s: %s\n", pr.Number, pr.Title)
	fmt.Printf("   %d files changed\n\n", len(pr.Files))

	// Dispatch agents.
	llmBackend := claude.New()
	orchestrator, orchErr := agents.NewOrchestrator(roles, agents.Options{
		Verbose: opts.verbose,
		DryRun:  opts.dryRun,
	}, llmBackend)
	if orchErr != nil {
		return fmt.Errorf("failed to initialize orchestrator: %w", orchErr)
	}

	start := time.Now()
	result, err := orchestrator.Review(pr)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}
	elapsed := time.Since(start)

	// Output the results.
	if err := outputResults(opts, pr, result, roles, elapsed); err != nil {
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
func outputResults(opts *reviewOptions, pr *gh.PR, result *agents.ReviewResult, roles []agents.Role, elapsed time.Duration) error {
	if opts.formatFlag == "" {
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
	ext := opts.formatFlag
	if ext == "markdown" {
		ext = "md"
	}

	switch opts.formatFlag {
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
		return fmt.Errorf("unknown format: %s (available: md, html, json)", opts.formatFlag)
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

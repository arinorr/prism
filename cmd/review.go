package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/report"
)

const defaultResultsDir = "results"

func runReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	var (
		prRef      = ""
		comment    = false
		verbose    = false
		dryRun     = false
		toStdout   = false
		rolesFlag  = ""
		formatFlag = ""
	)

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--comment":
			comment = true
		case args[i] == "--verbose" || args[i] == "-v":
			verbose = true
		case args[i] == "--dry-run":
			dryRun = true
		case args[i] == "--stdout":
			toStdout = true
		case args[i] == "--roles" && i+1 < len(args):
			i++
			rolesFlag = args[i]
		case strings.HasPrefix(args[i], "--roles="):
			rolesFlag = strings.TrimPrefix(args[i], "--roles=")
		case args[i] == "--format" && i+1 < len(args):
			i++
			formatFlag = args[i]
		case strings.HasPrefix(args[i], "--format="):
			formatFlag = strings.TrimPrefix(args[i], "--format=")
		case !strings.HasPrefix(args[i], "-"):
			prRef = args[i]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if prRef == "" {
		return fmt.Errorf("usage: prism review <pr-number|pr-url>")
	}

	// Determine which roles to use.
	roles := agents.AllRoles
	if rolesFlag != "" {
		var err error
		roles, err = agents.ParseRoles(rolesFlag)
		if err != nil {
			return err
		}
	}

	// Fetch PR diff.
	client, err := gh.NewClient()
	if err != nil {
		return fmt.Errorf("failed to initialize GitHub client: %w", err)
	}

	pr, err := client.GetPRDiff(prRef)
	if err != nil {
		return fmt.Errorf("failed to get PR diff: %w", err)
	}

	fmt.Printf("🥷 Reviewing PR #%s: %s\n", pr.Number, pr.Title)
	fmt.Printf("   %d files changed\n\n", len(pr.Files))

	// Dispatch agents.
	orchestrator, err := agents.NewOrchestrator(roles, agents.Options{
		Verbose: verbose,
		DryRun:  dryRun,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize orchestrator: %w", err)
	}

	start := time.Now()
	result, err := orchestrator.Review(pr)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}
	elapsed := time.Since(start)

	// If no format specified, print summary to terminal and we're done.
	if formatFlag == "" {
		fmt.Println(result.Summary)
	} else {
		// Build report data only when a format is requested.
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

		if toStdout {
			fmt.Print(output)
		} else {
			outPath := filepath.Join(defaultResultsDir, fmt.Sprintf("prism-pr-%s.%s", sanitizeFilename(pr.Number), ext))
			if err := writeToFile(output, outPath); err != nil {
				return err
			}
		}
	}

	// Optionally post comments.
	if comment {
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

// sanitizeFilename strips characters that could cause path traversal or invalid filenames.
func sanitizeFilename(s string) string {
	// Extract just the numeric part if it's a URL-style ref.
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		s = s[idx+1:]
	}
	// Remove any remaining path separators or suspicious characters.
	replacer := strings.NewReplacer("/", "-", "\\", "-", "..", "", "~", "")
	return replacer.Replace(s)
}

func writeToFile(content, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write output to %s: %w", path, err)
	}
	fmt.Printf("📄 Report written to %s\n", path)
	return nil
}

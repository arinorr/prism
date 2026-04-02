package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arinorr/shinobi/internal/agents"
	"github.com/arinorr/shinobi/internal/gh"
	"github.com/arinorr/shinobi/internal/report"
)

func runReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: shinobi review <pr-number|pr-url>")
	}

	var (
		prRef      = ""
		comment    = false
		verbose    = false
		dryRun     = false
		rolesFlag  = ""
		formatFlag = ""
		outputFlag = ""
	)

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--comment":
			comment = true
		case args[i] == "--verbose" || args[i] == "-v":
			verbose = true
		case args[i] == "--dry-run":
			dryRun = true
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
		case args[i] == "--output" || args[i] == "-o" && i+1 < len(args):
			i++
			outputFlag = args[i]
		case strings.HasPrefix(args[i], "--output="):
			outputFlag = strings.TrimPrefix(args[i], "--output=")
		case !strings.HasPrefix(args[i], "-"):
			prRef = args[i]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if prRef == "" {
		return fmt.Errorf("usage: shinobi review <pr-number|pr-url>")
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

	// Build report data.
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

	// Output based on format.
	switch formatFlag {
	case "md", "markdown":
		output := report.Markdown(data)
		if err := writeOutput(output, outputFlag); err != nil {
			return err
		}
	case "html":
		output, htmlErr := report.HTML(data)
		if htmlErr != nil {
			return fmt.Errorf("failed to generate HTML report: %w", htmlErr)
		}
		if err := writeOutput(output, outputFlag); err != nil {
			return err
		}
	case "json":
		output, jsonErr := report.JSON(data)
		if jsonErr != nil {
			return fmt.Errorf("failed to generate JSON report: %w", jsonErr)
		}
		if err := writeOutput(output, outputFlag); err != nil {
			return err
		}
	default:
		// Default: print summary to terminal.
		fmt.Println(result.Summary)
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

func writeOutput(content, path string) error {
	if path == "" {
		fmt.Print(content)
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write output to %s: %w", path, err)
	}
	fmt.Printf("📄 Report written to %s\n", path)
	return nil
}

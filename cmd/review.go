package cmd

import (
	"fmt"
	"strings"

	"github.com/arinorr/shinobi/internal/agents"
	"github.com/arinorr/shinobi/internal/gh"
)

func runReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: shinobi review <pr-number|pr-url>")
	}

	var (
		prRef     = ""
		comment   = false
		verbose   = false
		dryRun    = false
		rolesFlag = ""
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
	result, err := orchestrator.Review(pr)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	// Output synthesis.
	fmt.Println(result.Summary)

	// Optionally post comments.
	if comment {
		if err := client.PostComments(pr, result.Suggestions); err != nil {
			return fmt.Errorf("failed to post comments: %w", err)
		}
		fmt.Printf("\n✅ Posted %d inline comments to PR #%s\n", len(result.Suggestions), pr.Number)
	}

	return nil
}

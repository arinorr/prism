package cmd

import (
	"fmt"
	"io/fs"
	"os"
)

const version = "1.0.0"

// skillsFS holds the embedded skills directory. Set by main via SetSkillsFS.
var skillsFS fs.FS

// SetSkillsFS sets the embedded filesystem containing skill files.
// Called from main before Execute.
func SetSkillsFS(f fs.FS) {
	skillsFS = f
}

// Execute parses the command-line arguments and runs the appropriate subcommand.
func Execute() error {
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}

	switch os.Args[1] {
	case "review":
		return runReview(os.Args[2:])
	case "version":
		fmt.Printf("prism %s\n", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s\nRun 'prism help' for usage", os.Args[1])
	}
}

func printUsage() {
	fmt.Println(`prism - multi-agent PR review orchestrator

Usage:
  prism review <pr-number|pr-url>  Review a pull request
  prism version                    Print version
  prism help                       Show this help

Options:
  --comment        Post suggestions as inline PR comments (requires gh cli)
  --roles          Comma-separated list of roles to use (default: all)
                   Available: know-it-all,architect,solver,editor,optimizer,sentinel,test-engineer
  --format         Output format: plain, md, html, json (default: html)
                   Reports are saved to results/ directory
  --model          Claude model to use (e.g. sonnet, opus, haiku)
  --timeout        Per-agent timeout as a Go duration (default: 5m)
  --max-retries    Number of retries per agent on failure (default: 1)
  --max-budget-usd Maximum dollar spend per agent call (e.g. 0.50)
  --config         Path to config file (default: .prism.yml)
  --debate         Enable severity debate for high-disagreement findings
  --no-compress    Disable diff compression (send raw diff to agents)
  --stdout         Print report to terminal instead of saving to file
  -y, --yes        Skip confirmation prompt (auto-confirm estimate)
  -v, --verbose    Show detailed output (prompts, timing, raw responses)
  --dry-run        Show what would happen without calling Claude
  --estimate       Show estimated token usage and cost, then exit
  --cheap          Run all agents on Haiku (~$0.10/review)
  --verify         Enable post-dedup finding verification (Haiku + Opus)
  --no-verify      Disable verification
  --cross-refs     Enable cross-category reference injection
  --scope-hints    Enable scope hints (modified/added/existing symbols)`)
}

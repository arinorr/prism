package cmd

import (
	"fmt"
	"os"
)

const version = "0.1.0"

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
  --format         Output format: md, html, json (default: terminal summary)
                   Reports are saved to results/ directory
  --model          Claude model to use (e.g. sonnet, opus, haiku)
  --timeout        Per-agent timeout as a Go duration (default: 5m)
  --max-retries    Number of retries per agent on failure (default: 1)
  --config         Path to config file (default: .prism.yml)
  --stdout         Print report to terminal instead of saving to file
  -y, --yes        Skip confirmation prompts (e.g. large diff warning)
  -v, --verbose    Show detailed output (prompts, timing, raw responses)
  --dry-run        Show what would happen without calling Claude`)
}

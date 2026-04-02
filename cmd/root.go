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
		fmt.Printf("shinobi %s\n", version)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s\nRun 'shinobi help' for usage", os.Args[1])
	}
}

func printUsage() {
	fmt.Println(`shinobi - multi-agent PR review orchestrator

Usage:
  shinobi review <pr-number|pr-url>  Review a pull request
  shinobi version                    Print version
  shinobi help                       Show this help

Options:
  --comment    Post suggestions as inline PR comments (requires gh cli)
  --roles      Comma-separated list of roles to use (default: all)
               Available: know-it-all,architect,solver,editor,optimizer,sentinel`)
}

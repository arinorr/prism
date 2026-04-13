package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/arinorr/prism/cmd"
)

//go:embed skills/*
var skillsFS embed.FS

func main() {
	cmd.SetSkillsFS(skillsFS)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

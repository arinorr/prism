// Package claude implements the LLM interface for the Claude CLI.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/arinorr/prism/internal/llm"
)

// commandRunner executes a command and returns its output.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output() // #nosec G204 -- binary is hardcoded "claude"
}

// Adapter implements llm.LLM using the Claude CLI (`claude --print`).
type Adapter struct {
	run commandRunner
}

// New creates a new Claude CLI adapter.
func New() *Adapter {
	return &Adapter{run: defaultRunner}
}

// newWithRunner creates an adapter with a custom command runner (for testing).
func newWithRunner(run commandRunner) *Adapter {
	return &Adapter{run: run}
}

// Complete sends a prompt to Claude via the CLI and returns the response text.
// When req.JSONOutput is true, it passes --output-format json and unwraps the
// {"result": "..."} envelope that Claude CLI produces.
func (a *Adapter) Complete(ctx context.Context, req llm.Request) (string, error) {
	args := []string{"--print"}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	if req.JSONOutput {
		args = append(args, "--output-format", "json")
	}

	if req.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.SystemPrompt)
	}

	args = append(args, "-p", req.UserPrompt)

	out, err := a.run(ctx, "claude", args...)
	if err != nil {
		return "", fmt.Errorf("claude command failed: %w", err)
	}

	// When --output-format json is used, Claude wraps the response in
	// {"result": "..."}. Unwrap it to return clean text.
	if req.JSONOutput {
		return unwrapEnvelope(out)
	}

	// For non-JSON requests (like synthesis), try to unwrap the envelope
	// but fall back to raw output if it's not JSON.
	result, unwrapErr := unwrapEnvelope(out)
	if unwrapErr != nil {
		return string(out), nil
	}
	return result, nil
}

// unwrapEnvelope extracts the "result" field from Claude's JSON envelope.
func unwrapEnvelope(data []byte) (string, error) {
	var envelope struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("failed to parse claude response envelope: %w", err)
	}
	return envelope.Result, nil
}

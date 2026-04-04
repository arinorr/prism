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

// cliEnvelope is the full JSON response from `claude --print --output-format json`.
type cliEnvelope struct {
	Result       string  `json:"result"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	DurationMS   int     `json:"duration_ms"`
	Usage        struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// Complete sends a prompt to Claude via the CLI and returns the response text
// along with token usage metrics.
func (a *Adapter) Complete(ctx context.Context, req llm.Request) (string, llm.Usage, error) {
	// Always use JSON output to get usage metrics.
	args := []string{"--print", "--output-format", "json"}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	if req.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.SystemPrompt)
	}

	args = append(args, "-p", req.UserPrompt)

	out, err := a.run(ctx, "claude", args...)
	if err != nil {
		return "", llm.Usage{}, fmt.Errorf("claude command failed: %w", err)
	}

	var env cliEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		return "", llm.Usage{}, fmt.Errorf("failed to parse claude response envelope: %w", err)
	}

	usage := llm.Usage{
		InputTokens:              env.Usage.InputTokens,
		OutputTokens:             env.Usage.OutputTokens,
		CacheCreationInputTokens: env.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     env.Usage.CacheReadInputTokens,
		CostUSD:                  env.TotalCostUSD,
		DurationMS:               env.DurationMS,
	}

	return env.Result, usage, nil
}

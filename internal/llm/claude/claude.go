// Package claude implements the LLM interface for the Claude CLI.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/arinorr/prism/internal/llm"
)

// commandRunner executes a command and returns its output.
type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func defaultRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- binary is hardcoded "claude"
	// Unset CLAUDECODE so prism can be run from within a Claude Code session.
	// Without this, the Claude CLI refuses to start: "cannot be launched inside
	// another Claude Code session."
	cmd.Env = filterEnv(os.Environ(), "CLAUDECODE")
	// CombinedOutput captures both stdout and stderr so error messages
	// from the Claude CLI (rate limits, auth failures, etc.) are preserved.
	return cmd.CombinedOutput()
}

// filterEnv returns env without any variable matching the given prefix.
func filterEnv(env []string, prefix string) []string {
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix+"=") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// Adapter implements llm.LLM using the Claude CLI (`claude --print`).
type Adapter struct {
	run  commandRunner
	path string
}

// New creates a new Claude CLI adapter. path is the resolved path to the
// claude binary (either bare "claude" for PATH lookup, or an absolute path
// from probing common install locations).
func New(path string) *Adapter {
	return &Adapter{run: defaultRunner, path: path}
}

// newWithRunner creates an adapter with a custom command runner (for testing).
// Tests assert the binary name passed through; default to "claude".
func newWithRunner(run commandRunner) *Adapter {
	return &Adapter{run: run, path: "claude"}
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
	// Always use JSON output to get usage metrics. Disable MCP servers and
	// built-in tools: prism agents are pure prompt-in / JSON-out (they emit
	// findings as JSON, never need to shell out or edit files). Disabling
	// avoids shipping tool definitions in every request — saves tokens, avoids
	// MCP server startup latency per call, and immunizes agent calls from any
	// invalid tool schema in the user's MCP config.
	args := []string{
		"--print",
		"--output-format", "json",
		"--strict-mcp-config", // ignore all user-level MCP servers
		"--tools", "",         // disable all built-in tools (Bash, Edit, etc.)
	}

	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}

	if req.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", req.MaxBudgetUSD))
	}

	if req.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", req.SystemPrompt)
	}

	args = append(args, "-p", req.UserPrompt)

	out, err := a.run(ctx, a.path, args...)
	if err != nil {
		// If the command produced output before failing, it may contain
		// an error message from the Claude CLI. Include it in the error.
		if len(out) > 0 {
			return "", llm.Usage{}, fmt.Errorf("claude command failed: %w\nOutput: %s", err, truncate(out, 500))
		}
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

func truncate(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}

// Package llm defines the interface for language model backends.
//
// Prism uses the ports-and-adapters pattern: the orchestrator depends
// on the LLM interface (port), and concrete implementations like the
// Claude CLI adapter (adapter) satisfy it. This allows swapping LLM
// providers without changing business logic.
package llm

import "context"

// LLM is the port that the orchestrator uses to communicate with a
// language model. Implementations handle provider-specific details
// like CLI flags, API auth, and response envelope formats.
type LLM interface {
	// Complete sends a prompt to the language model and returns the
	// response text along with token usage metrics. The adapter handles
	// any provider-specific response wrapping (e.g., JSON envelopes)
	// and returns clean text.
	Complete(ctx context.Context, req Request) (string, Usage, error)
}

// Request contains the parameters for an LLM completion.
type Request struct {
	// SystemPrompt is prepended as a system-level instruction (e.g., a
	// reviewer skill file). Empty for synthesis calls.
	SystemPrompt string

	// UserPrompt is the main prompt content (e.g., the PR diff to review
	// or the synthesis instructions).
	UserPrompt string

	// JSONOutput requests that the model output valid JSON. The adapter
	// may use provider-specific mechanisms to enforce this.
	JSONOutput bool

	// Model overrides the default model for this request. Empty means
	// use the provider's default.
	Model string

	// MaxBudgetUSD caps the maximum dollar spend for this request.
	// Zero means no limit. Passed as --max-budget-usd to the Claude CLI.
	MaxBudgetUSD float64
}

// Usage tracks token consumption and cost for a single LLM call.
type Usage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	CostUSD                  float64 `json:"cost_usd"`
	DurationMS               int     `json:"duration_ms"`
}

// Add returns a new Usage that is the sum of u and other.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		InputTokens:              u.InputTokens + other.InputTokens,
		OutputTokens:             u.OutputTokens + other.OutputTokens,
		CacheCreationInputTokens: u.CacheCreationInputTokens + other.CacheCreationInputTokens,
		CacheReadInputTokens:     u.CacheReadInputTokens + other.CacheReadInputTokens,
		CostUSD:                  u.CostUSD + other.CostUSD,
		DurationMS:               u.DurationMS + other.DurationMS,
	}
}

// TotalTokens returns the total token count (input + output + cache).
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

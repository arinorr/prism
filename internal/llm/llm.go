// Package llm defines the interface for language model backends.
//
// Prism uses the ports-and-adapters pattern: the orchestrator depends
// on the LLM interface (port), and concrete implementations like the
// Claude CLI adapter (adapter) satisfy it. This allows swapping LLM
// providers without changing business logic.
package llm

import (
	"context"
	"sync"
)

// LLM is the port that the orchestrator uses to communicate with a
// language model. Implementations handle provider-specific details
// like CLI flags, API auth, and response envelope formats.
type LLM interface {
	// Complete sends a prompt to the language model and returns the
	// response text. The adapter handles any provider-specific response
	// wrapping (e.g., JSON envelopes) and returns clean text.
	Complete(ctx context.Context, req Request) (string, error)
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
}

// Mock is a test double for the LLM interface. It is safe for
// concurrent use (agents call Complete from goroutines).
type Mock struct {
	// Response is returned by Complete. Set this before calling.
	Response string

	// Err is returned by Complete if non-nil.
	Err error

	// Calls records each request passed to Complete.
	Calls []Request

	// CompleteFunc, if set, is called instead of returning Response/Err.
	// This allows per-call behavior in tests.
	CompleteFunc func(ctx context.Context, req Request) (string, error)

	mu sync.Mutex
}

// Complete satisfies the LLM interface.
func (m *Mock) Complete(ctx context.Context, req Request) (string, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, req)
	}
	return m.Response, m.Err
}

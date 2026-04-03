// Package llmtest provides test doubles for the llm.LLM interface.
//
// This package exists so that test mocks are not compiled into the
// production binary. Import it only from _test.go files.
package llmtest

import (
	"context"
	"sync"

	"github.com/arinorr/prism/internal/llm"
)

// Mock is a test double for the LLM interface. It is safe for
// concurrent use (agents call Complete from goroutines).
type Mock struct {
	// Response is returned by Complete. Set this before calling.
	Response string

	// Err is returned by Complete if non-nil.
	Err error

	// Calls records each request passed to Complete.
	Calls []llm.Request

	// CompleteFunc, if set, is called instead of returning Response/Err.
	// This allows per-call behavior in tests.
	CompleteFunc func(ctx context.Context, req llm.Request) (string, error)

	mu sync.Mutex
}

// Complete satisfies the llm.LLM interface.
func (m *Mock) Complete(ctx context.Context, req llm.Request) (string, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()
	if m.CompleteFunc != nil {
		return m.CompleteFunc(ctx, req)
	}
	return m.Response, m.Err
}

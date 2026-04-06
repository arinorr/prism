package llm_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
)

func TestMock_ReturnsResponse(t *testing.T) {
	t.Parallel()
	m := &llmtest.Mock{Response: "hello"}
	resp, _, err := m.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello" {
		t.Errorf("expected 'hello', got %q", resp)
	}
}

func TestMock_ReturnsError(t *testing.T) {
	t.Parallel()
	m := &llmtest.Mock{Err: fmt.Errorf("fail")}
	_, _, err := m.Complete(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMock_RecordsCalls(t *testing.T) {
	t.Parallel()
	m := &llmtest.Mock{Response: "ok"}
	_, _, _ = m.Complete(context.Background(), llm.Request{UserPrompt: "first"})
	_, _, _ = m.Complete(context.Background(), llm.Request{UserPrompt: "second"})
	if len(m.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(m.Calls))
	}
	if m.Calls[0].UserPrompt != "first" {
		t.Errorf("first call prompt: %q", m.Calls[0].UserPrompt)
	}
	if m.Calls[1].UserPrompt != "second" {
		t.Errorf("second call prompt: %q", m.Calls[1].UserPrompt)
	}
}

func TestMock_CompleteFunc(t *testing.T) {
	t.Parallel()
	m := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			return "custom: " + req.UserPrompt, llm.Usage{}, nil
		},
	}
	resp, _, err := m.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "custom: test" {
		t.Errorf("expected 'custom: test', got %q", resp)
	}
}

func TestMock_CompleteFuncOverridesResponse(t *testing.T) {
	t.Parallel()
	m := &llmtest.Mock{
		Response: "should not be returned",
		CompleteFunc: func(_ context.Context, _ llm.Request) (string, llm.Usage, error) {
			return "from func", llm.Usage{}, nil
		},
	}
	resp, _, _ := m.Complete(context.Background(), llm.Request{})
	if resp != "from func" {
		t.Errorf("CompleteFunc should override Response, got %q", resp)
	}
}

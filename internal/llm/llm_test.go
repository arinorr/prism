package llm

import (
	"context"
	"fmt"
	"testing"
)

func TestMock_ReturnsResponse(t *testing.T) {
	m := &Mock{Response: "hello"}
	resp, err := m.Complete(context.Background(), Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello" {
		t.Errorf("expected 'hello', got %q", resp)
	}
}

func TestMock_ReturnsError(t *testing.T) {
	m := &Mock{Err: fmt.Errorf("fail")}
	_, err := m.Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMock_RecordsCalls(t *testing.T) {
	m := &Mock{Response: "ok"}
	_, _ = m.Complete(context.Background(), Request{UserPrompt: "first"})
	_, _ = m.Complete(context.Background(), Request{UserPrompt: "second"})
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
	m := &Mock{
		CompleteFunc: func(_ context.Context, req Request) (string, error) {
			return "custom: " + req.UserPrompt, nil
		},
	}
	resp, err := m.Complete(context.Background(), Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "custom: test" {
		t.Errorf("expected 'custom: test', got %q", resp)
	}
}

func TestMock_CompleteFuncOverridesResponse(t *testing.T) {
	m := &Mock{
		Response: "should not be returned",
		CompleteFunc: func(_ context.Context, _ Request) (string, error) {
			return "from func", nil
		},
	}
	resp, _ := m.Complete(context.Background(), Request{})
	if resp != "from func" {
		t.Errorf("CompleteFunc should override Response, got %q", resp)
	}
}

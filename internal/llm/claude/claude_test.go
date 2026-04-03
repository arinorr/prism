package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/llm"
)

func envelope(result string) []byte {
	data, err := json.Marshal(struct {
		Result string `json:"result"`
	}{Result: result})
	if err != nil {
		panic(err)
	}
	return data
}

func TestComplete_JSONOutput(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		if name != "claude" {
			t.Errorf("expected 'claude', got %q", name)
		}
		// Verify --output-format json is passed.
		found := false
		for i, a := range args {
			if a == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
				found = true
			}
		}
		if !found {
			t.Error("expected --output-format json in args")
		}
		return envelope(`{"findings":[]}`), nil
	})

	resp, err := adapter.Complete(context.Background(), llm.Request{
		SystemPrompt: "You are a reviewer.",
		UserPrompt:   "Review this code.",
		JSONOutput:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != `{"findings":[]}` {
		t.Errorf("expected unwrapped findings, got %q", resp)
	}
}

func TestComplete_PlainOutput(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		// Verify --output-format json is NOT passed.
		for _, a := range args {
			if a == "--output-format" {
				t.Error("should not pass --output-format for non-JSON requests")
			}
		}
		return []byte("Overall looks good."), nil
	})

	resp, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "Synthesize feedback.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Overall looks good." {
		t.Errorf("expected raw text, got %q", resp)
	}
}

func TestComplete_PlainOutputWithEnvelope(t *testing.T) {
	// When Claude wraps even non-JSON requests in an envelope.
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		return envelope("Summary text here."), nil
	})

	resp, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "Synthesize.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Summary text here." {
		t.Errorf("expected unwrapped text, got %q", resp)
	}
}

func TestComplete_SystemPromptPassedAsFlag(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		found := false
		for i, a := range args {
			if a == "--append-system-prompt" && i+1 < len(args) {
				if args[i+1] == "skill content" {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected --append-system-prompt with skill content")
		}
		return envelope("ok"), nil
	})

	_, err := adapter.Complete(context.Background(), llm.Request{
		SystemPrompt: "skill content",
		UserPrompt:   "prompt",
		JSONOutput:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_NoSystemPromptOmitsFlag(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "--append-system-prompt" {
				t.Error("should not pass --append-system-prompt when SystemPrompt is empty")
			}
		}
		return []byte("ok"), nil
	})

	_, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "prompt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_CommandError(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("command not found")
	})

	_, err := adapter.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claude command failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestComplete_InvalidEnvelope(t *testing.T) {
	adapter := newWithRunner(func(name string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	})

	_, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "test",
		JSONOutput: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid envelope with JSONOutput=true")
	}
}

func TestUnwrapEnvelope_Valid(t *testing.T) {
	result, err := unwrapEnvelope(envelope("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestUnwrapEnvelope_Invalid(t *testing.T) {
	_, err := unwrapEnvelope([]byte("not json"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNew(t *testing.T) {
	adapter := New()
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.run == nil {
		t.Fatal("expected run to be set")
	}
}

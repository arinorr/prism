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
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, name string, args ...string) ([]byte, error) {
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

	resp, _, err := adapter.Complete(context.Background(), llm.Request{
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
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, name string, args ...string) ([]byte, error) {
		// Even non-JSON requests now use --output-format json for usage metrics.
		found := false
		for i, a := range args {
			if a == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
				found = true
			}
		}
		if !found {
			t.Error("expected --output-format json in args")
		}
		return envelope("Overall looks good."), nil
	})

	resp, _, err := adapter.Complete(context.Background(), llm.Request{
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
	t.Parallel()
	// When Claude wraps even non-JSON requests in an envelope.
	adapter := newWithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return envelope("Summary text here."), nil
	})

	resp, _, err := adapter.Complete(context.Background(), llm.Request{
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
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
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

	_, _, err := adapter.Complete(context.Background(), llm.Request{
		SystemPrompt: "skill content",
		UserPrompt:   "prompt",
		JSONOutput:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_NoSystemPromptOmitsFlag(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "--append-system-prompt" {
				t.Error("should not pass --append-system-prompt when SystemPrompt is empty")
			}
		}
		return envelope("ok"), nil
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "prompt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_CommandError(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, fmt.Errorf("command not found")
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claude command failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestComplete_InvalidEnvelope(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("not json"), nil
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "test",
		JSONOutput: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid envelope with JSONOutput=true")
	}
}

func TestComplete_ModelPassedAsFlag(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		found := false
		for i, a := range args {
			if a == "--model" && i+1 < len(args) && args[i+1] == "sonnet" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected --model sonnet in args, got %v", args)
		}
		return envelope("ok"), nil
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "prompt",
		Model:      "sonnet",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComplete_NoModelOmitsFlag(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "--model" {
				t.Error("should not pass --model when Model is empty")
			}
		}
		return envelope("ok"), nil
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{
		UserPrompt: "prompt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseEnvelope_Valid(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return envelope("hello"), nil
	})
	resp, _, err := adapter.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello" {
		t.Errorf("expected 'hello', got %q", resp)
	}
}

func TestParseEnvelope_Invalid(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("not json"), nil
	})
	_, _, err := adapter.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestComplete_DisablesToolsAndMCP ensures every agent call carries the
// flags that suppress tools + MCP server registration. A regression here
// would make prism vulnerable to invalid schemas in a user's MCP config
// (which would 400 the entire API request) and waste tokens shipping tool
// definitions agents never use.
func TestComplete_DisablesToolsAndMCP(t *testing.T) {
	t.Parallel()
	adapter := newWithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		hasStrictMCP := false
		toolsIdx := -1
		for i, a := range args {
			if a == "--strict-mcp-config" {
				hasStrictMCP = true
			}
			if a == "--tools" {
				toolsIdx = i
			}
		}
		if !hasStrictMCP {
			t.Error("expected --strict-mcp-config to be passed (disables user MCP servers)")
		}
		if toolsIdx == -1 || toolsIdx+1 >= len(args) || args[toolsIdx+1] != "" {
			t.Errorf(`expected --tools "" to disable all built-in tools, got args=%v`, args)
		}
		return envelope("ok"), nil
	})

	_, _, err := adapter.Complete(context.Background(), llm.Request{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	adapter := New()
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
		return
	}
	if adapter.run == nil {
		t.Fatal("expected run to be set")
	}
}

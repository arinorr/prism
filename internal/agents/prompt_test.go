package agents

import (
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestBuildAgentPrompt_BasicDiffOnly(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Test PR", Body: "Description", Diff: "full diff"}
	pctx := AgentPromptContext{Diff: "agent diff"}
	role := &Role{Name: "Test"}

	prompt := buildAgentPrompt(role, pr, pctx)

	if !strings.Contains(prompt, "<pr-diff>\nagent diff\n</pr-diff>") {
		t.Error("expected agent-specific diff in prompt, not pr.Diff")
	}
	if !strings.Contains(prompt, "Test PR") {
		t.Error("expected PR title in prompt")
	}
	if strings.Contains(prompt, "<cross-references>") {
		t.Error("should NOT contain cross-references block when empty")
	}
	if strings.Contains(prompt, "<scope-context>") {
		t.Error("should NOT contain scope-context block when empty")
	}
}

func TestBuildAgentPrompt_WithCrossRefs(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Test", Body: ""}
	pctx := AgentPromptContext{
		Diff:      "diff content",
		CrossRefs: "<cross-references>\nHandleRequest\n</cross-references>",
	}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)

	if !strings.Contains(prompt, "<cross-references>") {
		t.Error("expected cross-references block in prompt")
	}
	if !strings.Contains(prompt, "context only") {
		t.Error("expected cross-ref instruction text")
	}
}

func TestBuildAgentPrompt_WithScopeHints(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Test", Body: ""}
	pctx := AgentPromptContext{
		Diff:       "diff content",
		ScopeHints: "Symbols modified: HandleRequest (handler.go)",
	}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)

	if !strings.Contains(prompt, "<scope-context>") {
		t.Error("expected scope-context block in prompt")
	}
	if !strings.Contains(prompt, "HandleRequest") {
		t.Error("expected scope hint content")
	}
}

func TestBuildAgentPrompt_WithAll(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Full PR", Body: "All features"}
	pctx := AgentPromptContext{
		Diff:       "the diff",
		CrossRefs:  "<cross-references>refs</cross-references>",
		ScopeHints: "Symbols modified: Foo (bar.go)",
	}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)

	// All three sections should be present in order.
	diffIdx := strings.Index(prompt, "<pr-diff>")
	crossIdx := strings.Index(prompt, "<cross-references>")
	scopeIdx := strings.Index(prompt, "<scope-context>")
	jsonIdx := strings.Index(prompt, "JSON")

	if diffIdx < 0 || crossIdx < 0 || scopeIdx < 0 || jsonIdx < 0 {
		t.Fatal("expected all sections present")
	}
	if !(diffIdx < crossIdx && crossIdx < scopeIdx && scopeIdx < jsonIdx) {
		t.Error("sections should be in order: diff → cross-refs → scope → JSON instructions")
	}
}

func TestBuildAgentPrompt_EmptyPR(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{}
	pctx := AgentPromptContext{Diff: ""}

	// Should not panic with empty PR fields.
	prompt := buildAgentPrompt(&Role{}, pr, pctx)
	if !strings.Contains(prompt, "JSON") {
		t.Error("expected JSON instruction even with empty PR")
	}
}

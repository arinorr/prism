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
		t.Error("expected agent-specific diff in prompt")
	}
	if !strings.Contains(prompt, "Test PR") {
		t.Error("expected PR title in prompt")
	}
	// Instructions should NOT be in user prompt (moved to system prompt).
	if strings.Contains(prompt, `"findings"`) {
		t.Error("JSON format instructions should be in system prompt, not user prompt")
	}
	// Security warning should NOT be in user prompt.
	if strings.Contains(prompt, "UNTRUSTED") {
		t.Error("security warning should be in system prompt, not user prompt")
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

func TestBuildAgentPrompt_OrderMetadataBeforeDiff(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Full PR", Body: "All features"}
	pctx := AgentPromptContext{
		Diff:       "the diff",
		CrossRefs:  "<cross-references>refs</cross-references>",
		ScopeHints: "Symbols modified: Foo (bar.go)",
	}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)

	// Order should be: title → description → scope → cross-refs → diff
	titleIdx := strings.Index(prompt, "<pr-title>")
	descIdx := strings.Index(prompt, "<pr-description>")
	scopeIdx := strings.Index(prompt, "<scope-context>")
	crossIdx := strings.Index(prompt, "<cross-references>")
	diffIdx := strings.Index(prompt, "<pr-diff>")

	if titleIdx < 0 || descIdx < 0 || scopeIdx < 0 || crossIdx < 0 || diffIdx < 0 {
		t.Fatal("expected all sections present")
	}
	if titleIdx >= descIdx || descIdx >= scopeIdx || scopeIdx >= crossIdx || crossIdx >= diffIdx {
		t.Errorf("sections in wrong order: title(%d) → desc(%d) → scope(%d) → cross-refs(%d) → diff(%d)",
			titleIdx, descIdx, scopeIdx, crossIdx, diffIdx)
	}
}

func TestBuildAgentPrompt_DiffIsLast(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{Title: "Test", Body: "Desc"}
	pctx := AgentPromptContext{Diff: "the actual diff content"}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)

	diffIdx := strings.Index(prompt, "<pr-diff>")
	if diffIdx < 0 {
		t.Fatal("expected <pr-diff> in prompt")
	}
	afterDiff := prompt[diffIdx:]
	endTag := strings.Index(afterDiff, "</pr-diff>")
	remainder := strings.TrimSpace(afterDiff[endTag+len("</pr-diff>"):])
	if remainder != "" {
		t.Errorf("expected nothing after </pr-diff>, got %q", remainder)
	}
}

func TestBuildAgentPrompt_EmptyPR(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{}
	pctx := AgentPromptContext{Diff: ""}

	prompt := buildAgentPrompt(&Role{}, pr, pctx)
	if !strings.Contains(prompt, "<pr-diff>") {
		t.Error("expected diff section even with empty PR")
	}
}

func TestSharedSystemInstructions_ContainsExpectedContent(t *testing.T) {
	t.Parallel()
	if !strings.Contains(sharedSystemInstructions, "UNTRUSTED") {
		t.Error("shared instructions should contain security warning")
	}
	if !strings.Contains(sharedSystemInstructions, `"findings"`) {
		t.Error("shared instructions should contain JSON format")
	}
	if !strings.Contains(sharedSystemInstructions, "critical") {
		t.Error("shared instructions should contain risk guide")
	}
	if !strings.Contains(sharedSystemInstructions, "Quality over quantity") {
		t.Error("shared instructions should contain quality guide")
	}
}

func TestNormalizePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		check func(string) bool
		desc  string
	}{
		{
			"trims trailing whitespace",
			"line1   \nline2\t\n",
			func(s string) bool { return s == "line1\nline2\n" },
			"should strip trailing spaces/tabs",
		},
		{
			"normalizes CRLF",
			"line1\r\nline2\r\n",
			func(s string) bool { return !strings.Contains(s, "\r") },
			"should not contain \\r",
		},
		{
			"collapses triple newlines",
			"a\n\n\n\nb",
			func(s string) bool { return !strings.Contains(s, "\n\n\n") },
			"should not have 3+ consecutive newlines",
		},
		{
			"ensures trailing newline",
			"content",
			func(s string) bool { return strings.HasSuffix(s, "\n") },
			"should end with newline",
		},
		{
			"no double trailing newline",
			"content\n\n\n",
			func(s string) bool { return strings.HasSuffix(s, "content\n") },
			"should have exactly one trailing newline",
		},
		{
			"empty string",
			"",
			func(s string) bool { return s == "\n" },
			"empty input should produce single newline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizePrompt(tt.input)
			if !tt.check(got) {
				t.Errorf("%s: got %q", tt.desc, got)
			}
		})
	}
}

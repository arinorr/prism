package agents

import (
	"strings"
	"testing"

	"github.com/arinorr/shinobi/internal/gh"
)

func TestParseFeedback_DirectJSON(t *testing.T) {
	input := `{"findings": [{"file": "main.go", "line": 10, "severity": "warning", "summary": "test", "detail": "detail"}]}`
	fb, err := parseFeedback("test-role", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb.Role != "test-role" {
		t.Errorf("expected role 'test-role', got %q", fb.Role)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	f := fb.Findings[0]
	if f.File != "main.go" || f.Line != 10 || f.Severity != "warning" {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestParseFeedback_MarkdownCodeBlock(t *testing.T) {
	input := "Here are my findings:\n```json\n" +
		`{"findings": [{"file": "foo.go", "line": 1, "severity": "info", "summary": "s", "detail": "d"}]}` +
		"\n```\nHope this helps!"
	fb, err := parseFeedback("editor", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].File != "foo.go" {
		t.Errorf("expected file 'foo.go', got %q", fb.Findings[0].File)
	}
}

func TestParseFeedback_ProseWrappedJSON(t *testing.T) {
	input := `Now I have analyzed the code thoroughly.

{"findings": [{"file": "cmd/root.go", "line": 5, "severity": "critical", "summary": "bug", "detail": "details here"}]}

That concludes my review.`
	fb, err := parseFeedback("solver", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", fb.Findings[0].Severity)
	}
}

func TestParseFeedback_EmptyFindings(t *testing.T) {
	input := `{"findings": []}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_InvalidJSON(t *testing.T) {
	input := "This is not JSON at all, just plain text without any braces."
	_, err := parseFeedback("test", input)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "could not extract JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseFeedback_MultipleCodeBlocks(t *testing.T) {
	input := "First block is not JSON:\n```\nsome text\n```\n\nSecond block has it:\n```json\n" +
		`{"findings": [{"file": "a.go", "line": 1, "severity": "info", "summary": "s", "detail": "d"}]}` +
		"\n```"
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
}

func TestTruncateUTF8_Short(t *testing.T) {
	s := "hello"
	got := truncateUTF8(s, 10)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_ExactLength(t *testing.T) {
	s := "hello"
	got := truncateUTF8(s, 5)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_Truncates(t *testing.T) {
	s := "hello world"
	got := truncateUTF8(s, 5)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_MultiByte(t *testing.T) {
	// "é" is 2 bytes in UTF-8. Cutting at byte 1 should back up.
	s := "é"
	got := truncateUTF8(s, 1)
	if got != "" {
		t.Errorf("expected empty string (can't fit the rune), got %q", got)
	}
}

func TestTruncateUTF8_MultiBytePreserved(t *testing.T) {
	// "aé" = 'a' (1 byte) + 'é' (2 bytes) = 3 bytes total.
	s := "aé"
	got := truncateUTF8(s, 3)
	if got != "aé" {
		t.Errorf("expected 'aé', got %q", got)
	}
	// Truncate at 2 bytes should keep just 'a'.
	got = truncateUTF8(s, 2)
	if got != "a" {
		t.Errorf("expected 'a', got %q", got)
	}
}

func TestBuildAgentPrompt(t *testing.T) {
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{
		Title: "Fix bug",
		Body:  "This fixes the bug",
		Diff:  "+ added line",
	}
	prompt := buildAgentPrompt(role, pr)
	if !strings.Contains(prompt, "Fix bug") {
		t.Error("prompt should contain PR title")
	}
	if !strings.Contains(prompt, "This fixes the bug") {
		t.Error("prompt should contain PR body")
	}
	if !strings.Contains(prompt, "+ added line") {
		t.Error("prompt should contain diff")
	}
	if !strings.Contains(prompt, `{"findings"`) {
		t.Error("prompt should contain output format instructions")
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	pr := &gh.PR{Title: "Test PR"}
	feedbacks := []Feedback{
		{Role: "know-it-all", Findings: []Finding{{File: "a.go", Severity: "info", Summary: "test"}}},
		{Role: "editor", Findings: []Finding{{File: "b.go", Severity: "warning", Summary: "test2"}}},
	}
	prompt := buildSynthesisPrompt(pr, feedbacks)
	if !strings.Contains(prompt, "Test PR") {
		t.Error("synthesis prompt should contain PR title")
	}
	if !strings.Contains(prompt, "know-it-all") {
		t.Error("synthesis prompt should contain agent feedback")
	}
	if !strings.Contains(prompt, "editor") {
		t.Error("synthesis prompt should contain all agents' feedback")
	}
}

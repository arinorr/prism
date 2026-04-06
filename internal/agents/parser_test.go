package agents

import (
	"strings"
	"testing"
)

// Parse strategy tests — direct JSON, markdown code blocks, prose-wrapped.

func TestParseFeedback_DirectJSON(t *testing.T) {
	t.Parallel()
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
	if f.File != "main.go" || f.Line != 10 || f.Risk != "warning" {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestParseFeedback_MarkdownCodeBlock(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	if fb.Findings[0].Risk != "critical" {
		t.Errorf("expected severity 'critical', got %q", fb.Findings[0].Risk)
	}
}

func TestParseFeedback_EmptyFindings(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Malformed response edge cases.

func TestParseFeedback_TruncatedJSON(t *testing.T) {
	t.Parallel()
	_, err := parseFeedback("test", `{"findings": [{"file": "a.go", "line": 10, "risk"`)
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
}

func TestParseFeedback_WrongSchemaReturnsEmpty(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"errors": ["something went wrong"]}`)
	if err != nil {
		t.Fatalf("wrong schema should parse as empty findings: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_NullFindingsReturnsEmpty(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings": null}`)
	if err != nil {
		t.Fatalf("null findings should parse as empty: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_ExtraFieldsForwardCompat(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"ok","detail":"d","new_field":"v"}],"metadata":{}}`)
	if err != nil {
		t.Fatalf("extra fields should not error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_EmptyResponse(t *testing.T) {
	t.Parallel()
	_, err := parseFeedback("test", "")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestParseFeedback_WhitespaceOnlyResponse(t *testing.T) {
	t.Parallel()
	_, err := parseFeedback("test", "   \n\t  ")
	if err == nil {
		t.Fatal("expected error for whitespace-only response")
	}
}

// Confidence filtering.

func TestParseFeedback_NegativeConfidenceClamped(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":-0.5}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("negative confidence finding should be filtered, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_ConfidenceExactlyAtThreshold(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":0.5}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Errorf("confidence at threshold should pass, got %d findings", len(fb.Findings))
	}
}

func TestParseFeedback_ConfidenceJustBelowThreshold(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":0.49}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("confidence below threshold should be filtered, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_SeverityBackwardCompat(t *testing.T) {
	t.Parallel()
	fb, err := parseFeedback("test", `{"findings":[{"file":"a.go","line":1,"severity":"warning","summary":"s","detail":"d"}]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].Risk != RiskWarning {
		t.Errorf("expected risk 'warning' from severity field, got %q", fb.Findings[0].Risk)
	}
}

// UTF-8 truncation.

func TestTruncateUTF8_Short(t *testing.T) {
	t.Parallel()
	if got := truncateUTF8("hello", 10); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_ExactLength(t *testing.T) {
	t.Parallel()
	if got := truncateUTF8("hello", 5); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_Truncates(t *testing.T) {
	t.Parallel()
	if got := truncateUTF8("hello world", 5); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_MultiByte(t *testing.T) {
	t.Parallel()
	if got := truncateUTF8("é", 1); got != "" {
		t.Errorf("expected empty string (can't fit the rune), got %q", got)
	}
}

func TestTruncateUTF8_MultiBytePreserved(t *testing.T) {
	t.Parallel()
	if got := truncateUTF8("aé", 3); got != "aé" {
		t.Errorf("expected 'aé', got %q", got)
	}
	if got := truncateUTF8("aé", 2); got != "a" {
		t.Errorf("expected 'a', got %q", got)
	}
}

// NormalizeFinding edge cases.

func TestNormalizeFinding_InvalidRiskDefaultsToInfo(t *testing.T) {
	t.Parallel()
	f := Finding{Risk: "unknown_risk"}
	NormalizeFinding(&f, "")
	if f.Risk != RiskInfo {
		t.Errorf("invalid risk should default to info, got %q", f.Risk)
	}
}

func TestNormalizeFinding_InvalidScopeDefaultsToChanged(t *testing.T) {
	t.Parallel()
	f := Finding{Risk: RiskInfo, Scope: "invalid_scope"}
	NormalizeFinding(&f, "")
	if f.Scope != ScopeChanged {
		t.Errorf("invalid scope should default to changed, got %q", f.Scope)
	}
}

func TestNormalizeFinding_MissingConfidenceGetsDefault(t *testing.T) {
	t.Parallel()
	f := Finding{Risk: RiskInfo, Confidence: 0}
	NormalizeFinding(&f, "")
	if f.Confidence != confidenceDefault {
		t.Errorf("missing confidence should default to %.1f, got %.1f", confidenceDefault, f.Confidence)
	}
}

package agents

import (
	"testing"
)

// Parser edge cases.

func TestParseFeedback_TruncatedJSON(t *testing.T) {
	t.Parallel()
	input := `{"findings": [{"file": "a.go", "line": 10, "risk"`
	_, err := parseFeedback("test", input)
	if err == nil {
		t.Fatal("expected error for truncated JSON")
	}
}

func TestParseFeedback_WrongSchemaReturnsEmpty(t *testing.T) {
	t.Parallel()
	// Valid JSON but no "findings" key — parses successfully with 0 findings.
	// This is correct: the agent found nothing.
	input := `{"errors": ["something went wrong"]}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("wrong schema should parse as empty findings: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_NullFindingsReturnsEmpty(t *testing.T) {
	t.Parallel()
	// null findings parses as empty array — agent found nothing.
	input := `{"findings": null}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("null findings should parse as empty: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_ExtraFieldsForwardCompat(t *testing.T) {
	t.Parallel()
	input := `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"ok","detail":"d","new_field":"v"}],"metadata":{}}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("extra fields should not error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_NegativeConfidenceClamped(t *testing.T) {
	t.Parallel()
	input := `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":-0.5}]}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Negative confidence is clamped to 0, which is below threshold — should be filtered.
	if len(fb.Findings) != 0 {
		t.Errorf("negative confidence finding should be filtered, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_ConfidenceExactlyAtThreshold(t *testing.T) {
	t.Parallel()
	input := `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":0.5}]}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Confidence == 0.5 should pass (threshold is >=).
	if len(fb.Findings) != 1 {
		t.Errorf("confidence at threshold should pass, got %d findings", len(fb.Findings))
	}
}

func TestParseFeedback_ConfidenceJustBelowThreshold(t *testing.T) {
	t.Parallel()
	input := `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"s","detail":"d","confidence":0.49}]}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("confidence below threshold should be filtered, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_SeverityBackwardCompat(t *testing.T) {
	t.Parallel()
	// Old "severity" field should be mapped to "risk".
	input := `{"findings":[{"file":"a.go","line":1,"severity":"warning","summary":"s","detail":"d"}]}`
	fb, err := parseFeedback("test", input)
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

// Dedup edge cases.

func TestDeduplicate_SameLineSameFile(t *testing.T) {
	t.Parallel()
	// Two findings at exactly the same line should merge.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Nil pointer dereference", Detail: "d", Role: "sentinel"},
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Possible nil dereference", Detail: "d", Role: "solver"},
	}
	deduped := Deduplicate(findings, 2)
	if len(deduped) != 1 {
		t.Errorf("same line + same category + similar summary should merge, got %d", len(deduped))
	}
	if len(deduped) > 0 && deduped[0].VoteCount != 2 {
		t.Errorf("expected 2 votes, got %d", deduped[0].VoteCount)
	}
}

func TestDeduplicate_EmptySummary(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskInfo, Category: "style", Scope: ScopeChanged, Summary: "", Detail: "d1", Role: "editor"},
		{File: "a.go", Line: 10, Risk: RiskInfo, Category: "style", Scope: ScopeChanged, Summary: "", Detail: "d2", Role: "sentinel"},
	}
	// Should not panic even with empty summaries.
	deduped := Deduplicate(findings, 2)
	if len(deduped) == 0 {
		t.Error("expected at least 1 deduped finding even with empty summaries")
	}
}

func TestDeduplicate_SingleFindingWithTotalAgents(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{File: "a.go", Line: 1, Risk: RiskInfo, Category: "style", Scope: ScopeChanged, Summary: "test", Detail: "d", Role: "editor"},
	}
	deduped := Deduplicate(findings, 7)
	if len(deduped) != 1 {
		t.Errorf("single finding should produce 1 deduped, got %d", len(deduped))
	}
	if deduped[0].VoteCount != 1 {
		t.Errorf("single finding should have 1 vote, got %d", deduped[0].VoteCount)
	}
	if deduped[0].TotalAgents != 7 {
		t.Errorf("total agents should be 7, got %d", deduped[0].TotalAgents)
	}
}

func TestDeduplicate_DifferentFilesSameIssue(t *testing.T) {
	t.Parallel()
	// Same summary but different files should NOT merge.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Missing nil check", Detail: "d", Role: "sentinel"},
		{File: "b.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Missing nil check", Detail: "d", Role: "solver"},
	}
	deduped := Deduplicate(findings, 2)
	if len(deduped) != 2 {
		t.Errorf("different files should not merge, got %d deduped", len(deduped))
	}
}

func TestDeduplicate_NilAndEmptySlice(t *testing.T) {
	t.Parallel()
	deduped := Deduplicate(nil, 7)
	if len(deduped) != 0 {
		t.Errorf("nil findings should produce empty deduped, got %d", len(deduped))
	}
	deduped = Deduplicate([]Finding{}, 7)
	if len(deduped) != 0 {
		t.Errorf("empty findings should produce empty deduped, got %d", len(deduped))
	}
}

// Health score edge cases.

func TestHealthScore_NoFindings(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 {
		t.Errorf("no findings should give 100, got %d", score.Score)
	}
	if score.Grade != "A+" {
		t.Errorf("no findings should give A+, got %q", score.Grade)
	}
}

func TestHealthScore_ManyHighConsensusFindings(t *testing.T) {
	t.Parallel()
	// 5 critical findings all with unanimous consensus — score should floor at 0.
	findings := make([]DedupedFinding, 5)
	for i := range findings {
		findings[i] = DedupedFinding{
			Finding:     Finding{Risk: RiskCritical, Scope: ScopeChanged},
			VoteCount:   7,
			TotalAgents: 7,
		}
	}
	score := ComputeHealthScore(findings)
	if score.Score != 0 {
		t.Errorf("5 unanimous criticals should floor at 0, got %d", score.Score)
	}
	if score.Grade != "F" {
		t.Errorf("expected grade F, got %q", score.Grade)
	}
	if score.Verdict != VerdictDiscuss {
		t.Errorf("expected verdict %q, got %q", VerdictDiscuss, score.Verdict)
	}
}

func TestHealthScore_LowConsensusReducesImpact(t *testing.T) {
	t.Parallel()
	// A critical finding with 1/7 votes should penalize less than 7/7.
	low := ComputeHealthScore([]DedupedFinding{
		{Finding: Finding{Risk: RiskCritical, Scope: ScopeChanged}, VoteCount: 1, TotalAgents: 7},
	})
	high := ComputeHealthScore([]DedupedFinding{
		{Finding: Finding{Risk: RiskCritical, Scope: ScopeChanged}, VoteCount: 7, TotalAgents: 7},
	})
	if low.Score <= high.Score {
		t.Errorf("low consensus (%d) should score higher than high consensus (%d)", low.Score, high.Score)
	}
}

// Normalize edge cases.

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

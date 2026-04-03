package agents

import (
	"testing"
)

func TestDeduplicate_IdenticalFindings(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "solver"},
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "optimizer"},
	}
	result := Deduplicate(findings, 7)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding, got %d", len(result))
	}
	if result[0].VoteCount != 3 {
		t.Errorf("expected vote count 3, got %d", result[0].VoteCount)
	}
	if result[0].TotalAgents != 7 {
		t.Errorf("expected total agents 7, got %d", result[0].TotalAgents)
	}
	if len(result[0].Voters) != 3 {
		t.Errorf("expected 3 voters, got %d", len(result[0].Voters))
	}
}

func TestDeduplicate_CloseLinesSimilarSummary(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "no timeout on scan operation", Role: "architect"},
		{File: "a.go", Line: 12, Severity: "critical", Summary: "missing timeout on scan operation", Role: "sentinel"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding, got %d", len(result))
	}
	if result[0].VoteCount != 2 {
		t.Errorf("expected vote count 2, got %d", result[0].VoteCount)
	}
	// Should keep the highest severity.
	if result[0].Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", result[0].Severity)
	}
}

func TestDeduplicate_DistantLinesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 100, Severity: "warning", Summary: "missing timeout", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (distant lines), got %d", len(result))
	}
}

func TestDeduplicate_DifferentSummariesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 10, Severity: "warning", Summary: "unused variable detected", Role: "editor"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (different summaries), got %d", len(result))
	}
}

func TestDeduplicate_DifferentFilesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "b.go", Line: 10, Severity: "warning", Summary: "missing timeout", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (different files), got %d", len(result))
	}
}

func TestDeduplicate_SingleFinding(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "info", Summary: "consider renaming", Role: "editor"},
	}
	result := Deduplicate(findings, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].VoteCount != 1 {
		t.Errorf("expected vote count 1, got %d", result[0].VoteCount)
	}
}

func TestDeduplicate_Empty(t *testing.T) {
	result := Deduplicate(nil, 5)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

func TestDeduplicate_SortOrder(t *testing.T) {
	findings := []Finding{
		{File: "b.go", Line: 1, Severity: "info", Summary: "style note", Role: "editor"},
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout on operation", Role: "architect"},
		{File: "a.go", Line: 10, Severity: "critical", Summary: "no timeout on operation", Role: "sentinel"},
		{File: "a.go", Line: 12, Severity: "warning", Summary: "missing timeout on the operation", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	// The three similar findings should merge; the info finding stays separate.
	if len(result) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result))
	}
	// First group should have highest vote count.
	if result[0].VoteCount != 3 {
		t.Errorf("first group should have 3 votes, got %d", result[0].VoteCount)
	}
	if result[1].VoteCount != 1 {
		t.Errorf("second group should have 1 vote, got %d", result[1].VoteCount)
	}
}

func TestDeduplicate_KeepsBestDetail(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Detail: "short", Role: "a"},
		{File: "a.go", Line: 10, Severity: "warning", Summary: "missing timeout", Detail: "a much longer and more detailed explanation of the issue", Role: "b"},
	}
	result := Deduplicate(findings, 2)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if result[0].Detail != "a much longer and more detailed explanation of the issue" {
		t.Errorf("expected longer detail to be kept, got %q", result[0].Detail)
	}
}

func TestJaccardSimilarity_Identical(t *testing.T) {
	s := jaccardSimilarity("missing timeout on scan", "missing timeout on scan")
	if s != 1.0 {
		t.Errorf("expected 1.0, got %f", s)
	}
}

func TestJaccardSimilarity_Similar(t *testing.T) {
	s := jaccardSimilarity("missing timeout on scan operation", "no timeout on scan operation")
	// "missing" and "no" differ, rest overlap: 3/5 = 0.6.
	if s < jaccardThreshold {
		t.Errorf("expected above threshold, got %f", s)
	}
}

func TestJaccardSimilarity_Different(t *testing.T) {
	s := jaccardSimilarity("missing timeout", "unused variable detected")
	if s >= jaccardThreshold {
		t.Errorf("expected below threshold for different summaries, got %f", s)
	}
}

func TestJaccardSimilarity_BothEmpty(t *testing.T) {
	s := jaccardSimilarity("", "")
	if s != 1.0 {
		t.Errorf("expected 1.0 for both empty, got %f", s)
	}
}

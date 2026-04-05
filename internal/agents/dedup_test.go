package agents

import (
	"testing"
)

func TestDeduplicate_IdenticalFindings(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "solver"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "optimizer"},
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
		{File: "a.go", Line: 10, Risk: "warning", Summary: "no timeout on scan operation", Role: "architect"},
		{File: "a.go", Line: 12, Risk: "critical", Summary: "missing timeout on scan operation", Role: "sentinel"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding, got %d", len(result))
	}
	if result[0].VoteCount != 2 {
		t.Errorf("expected vote count 2, got %d", result[0].VoteCount)
	}
	// Composite severity: critical (weight 2.0) + warning (weight 1.0) = 2.67 → critical.
	if result[0].Risk != "critical" {
		t.Errorf("expected composite severity 'critical', got %q", result[0].Risk)
	}
}

func TestDeduplicate_DistantLinesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 100, Risk: "warning", Summary: "missing timeout", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (distant lines), got %d", len(result))
	}
}

func TestDeduplicate_DifferentSummariesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "unused variable detected", Role: "editor"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (different summaries), got %d", len(result))
	}
}

func TestDeduplicate_DifferentFilesNotMerged(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "b.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 deduped findings (different files), got %d", len(result))
	}
}

func TestDeduplicate_SingleFinding(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "info", Summary: "consider renaming", Role: "editor"},
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
		{File: "b.go", Line: 1, Risk: "info", Summary: "style note", Role: "editor"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout on operation", Role: "architect"},
		{File: "a.go", Line: 10, Risk: "critical", Summary: "no timeout on operation", Role: "sentinel"},
		{File: "a.go", Line: 12, Risk: "warning", Summary: "missing timeout on the operation", Role: "solver"},
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
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Detail: "short", Role: "a"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Detail: "a much longer and more detailed explanation of the issue", Role: "b"},
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

func TestDeduplicate_PreservesAgentDetails(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Detail: "architect view", CodeExample: "// add timeout", Role: "architect"},
		{File: "a.go", Line: 10, Risk: "critical", Summary: "missing timeout", Detail: "sentinel view is much longer and more detailed", CodeExample: "ctx, cancel := ...", Role: "sentinel"},
		{File: "a.go", Line: 12, Risk: "warning", Summary: "missing timeout on scan", Detail: "solver view", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding, got %d", len(result))
	}
	if len(result[0].AgentDetails) != 3 {
		t.Fatalf("expected 3 agent details, got %d", len(result[0].AgentDetails))
	}
	// Best detail (longest) should be in the top-level Finding.
	if result[0].Detail != "sentinel view is much longer and more detailed" {
		t.Errorf("expected longest detail in top-level, got %q", result[0].Detail)
	}
	// Best code example (longest) should be in the top-level Finding.
	if result[0].CodeExample != "ctx, cancel := ..." {
		t.Errorf("expected longest code example, got %q", result[0].CodeExample)
	}
	// Each agent's detail preserved.
	roles := map[string]bool{}
	for _, ad := range result[0].AgentDetails {
		roles[ad.Role] = true
	}
	if !roles["architect"] || !roles["sentinel"] || !roles["solver"] {
		t.Errorf("expected all three agents in details, got %v", roles)
	}
}

func TestDeduplicate_SingleFindingHasOneAgentDetail(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "info", Summary: "note", Detail: "some detail", Role: "editor"},
	}
	result := Deduplicate(findings, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if len(result[0].AgentDetails) != 1 {
		t.Fatalf("expected 1 agent detail, got %d", len(result[0].AgentDetails))
	}
	if result[0].AgentDetails[0].Role != "editor" {
		t.Errorf("expected role 'editor', got %q", result[0].AgentDetails[0].Role)
	}
	if result[0].AgentDetails[0].Detail != "some detail" {
		t.Errorf("expected detail preserved, got %q", result[0].AgentDetails[0].Detail)
	}
}

func TestJaccardSimilarity_BothEmpty(t *testing.T) {
	s := jaccardSimilarity("", "")
	if s != 1.0 {
		t.Errorf("expected 1.0 for both empty, got %f", s)
	}
}

func TestComputeCompositeSeverity_AllSameRisk(t *testing.T) {
	details := []AgentDetail{
		{Role: "architect", Risk: "warning"},
		{Role: "editor", Risk: "warning"},
		{Role: "solver", Risk: "warning"},
	}
	cs := computeCompositeSeverity(details, "design")
	if cs != 2.0 {
		t.Errorf("expected 2.0 for all-warning, got %f", cs)
	}
}

func TestComputeCompositeSeverity_CriticalGravity(t *testing.T) {
	// One critical among info votes — critical gravity should keep it elevated.
	details := []AgentDetail{
		{Role: "sentinel", Risk: "critical"},
		{Role: "editor", Risk: "info"},
		{Role: "solver", Risk: "info"},
		{Role: "optimizer", Risk: "info"},
	}
	// sentinel critical: weight=2.0, value=3.0*2.0=6.0
	// 3 info agents: weight=3*1.0=3.0, value=3*1.0*1.0=3.0
	// composite = 9.0/5.0 = 1.8 → warning (not info, thanks to gravity)
	cs := computeCompositeSeverity(details, "design")
	label := compositeSeverityToLabel(cs)
	if label != "warning" {
		t.Errorf("expected critical gravity to keep composite at warning, got %q (%.2f)", label, cs)
	}
}

func TestComputeCompositeSeverity_DomainAuthorityBoost(t *testing.T) {
	// Sentinel (domain authority for security) says critical on a security finding.
	// Others say info. Domain authority boost should increase weight further.
	details := []AgentDetail{
		{Role: "sentinel", Risk: "critical"}, // weight: 2.0 (gravity) * 1.5 (authority) = 3.0
		{Role: "editor", Risk: "info"},       // weight: 1.0
		{Role: "solver", Risk: "info"},       // weight: 1.0
	}
	// sentinel: 3.0 * 3.0 = 9.0
	// editor: 1.0 * 1.0 = 1.0
	// solver: 1.0 * 1.0 = 1.0
	// composite = 11.0 / 5.0 = 2.2 → warning
	cs := computeCompositeSeverity(details, "security")
	if cs < 2.0 {
		t.Errorf("expected domain authority to boost composite above 2.0, got %f", cs)
	}
}

func TestCompositeSeverityToLabel_Thresholds(t *testing.T) {
	tests := []struct {
		cs    float64
		label string
	}{
		{3.0, "critical"},
		{2.5, "critical"},
		{2.49, "warning"},
		{1.7, "warning"},
		{1.69, "info"},
		{1.0, "info"},
	}
	for _, tt := range tests {
		got := compositeSeverityToLabel(tt.cs)
		if got != tt.label {
			t.Errorf("compositeSeverityToLabel(%.2f) = %q, want %q", tt.cs, got, tt.label)
		}
	}
}

func TestDisagreementSpread(t *testing.T) {
	tests := []struct {
		name    string
		details []AgentDetail
		spread  int
	}{
		{"single agent", []AgentDetail{{Risk: "critical"}}, 0},
		{"all agree", []AgentDetail{{Risk: "warning"}, {Risk: "warning"}}, 0},
		{"warning vs info", []AgentDetail{{Risk: "warning"}, {Risk: "info"}}, 1},
		{"critical vs info", []AgentDetail{{Risk: "critical"}, {Risk: "info"}}, 2},
		{"critical vs warning", []AgentDetail{{Risk: "critical"}, {Risk: "warning"}}, 1},
	}
	for _, tt := range tests {
		got := DisagreementSpread(tt.details)
		if got != tt.spread {
			t.Errorf("%s: got spread %d, want %d", tt.name, got, tt.spread)
		}
	}
}

func TestDeduplicate_SetsConfidenceAndCompositeSeverity(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "architect"},
		{File: "a.go", Line: 10, Risk: "warning", Summary: "missing timeout", Role: "solver"},
	}
	result := Deduplicate(findings, 7)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	// Confidence = 2/7.
	expectedConf := 2.0 / 7.0
	if diff := result[0].Confidence - expectedConf; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected confidence ~%.3f, got %.3f", expectedConf, result[0].Confidence)
	}
	// Both say warning, so composite = 2.0.
	if result[0].CompositeSeverity != 2.0 {
		t.Errorf("expected composite severity 2.0, got %f", result[0].CompositeSeverity)
	}
}

func TestDeduplicate_AgentDetailPreservesRisk(t *testing.T) {
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "critical", Summary: "missing timeout", Role: "sentinel"},
		{File: "a.go", Line: 10, Risk: "info", Summary: "missing timeout", Role: "editor"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1, got %d", len(result))
	}
	if len(result[0].AgentDetails) != 2 {
		t.Fatalf("expected 2 agent details, got %d", len(result[0].AgentDetails))
	}
	// Each agent's original risk should be preserved.
	risks := map[string]string{}
	for _, ad := range result[0].AgentDetails {
		risks[ad.Role] = ad.Risk
	}
	if risks["sentinel"] != "critical" {
		t.Errorf("sentinel risk should be critical, got %q", risks["sentinel"])
	}
	if risks["editor"] != "info" {
		t.Errorf("editor risk should be info, got %q", risks["editor"])
	}
}

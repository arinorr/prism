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
	// Should keep the highest severity.
	if result[0].Risk != "critical" {
		t.Errorf("expected severity 'critical', got %q", result[0].Risk)
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

// Hybrid dedup tests.

func TestDeduplicate_HybridTier1_SameFileCategoryNearbyLines(t *testing.T) {
	// Same file, same category, lines within wideLineThreshold (20).
	// Summaries use different wording but prefix stemming creates enough
	// overlap to exceed tier 1's 0.05 threshold.
	findings := []Finding{
		{File: "report.go", Line: 961, Risk: "warning", Category: "design",
			Summary: "Code duplication: countRawSeverity and countDedupedSeverity are identical implementations", Role: "architect"},
		{File: "report.go", Line: 967, Risk: "warning", Category: "design",
			Summary: "Duplicated severity counting and sorting logic across three functions", Role: "editor"},
	}
	result := Deduplicate(findings, 7)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 1: same file+category+nearby), got %d", len(result))
	}
	if result[0].VoteCount != 2 {
		t.Errorf("expected vote count 2, got %d", result[0].VoteCount)
	}
}

func TestDeduplicate_HybridTier2_SameFileCategoryDistantLines(t *testing.T) {
	// Same file, same category, lines beyond wideLineThreshold (20) but
	// within sameCategoryLineLimit (100). Tier 2 threshold of 0.15 applies.
	findings := []Finding{
		{File: "report.go", Line: 10, Risk: "info", Category: "design",
			Summary: "Duplicated severity counting logic across functions", Role: "architect"},
		{File: "report.go", Line: 80, Risk: "info", Category: "design",
			Summary: "Duplicate counting and sorting logic across three locations", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 2: same file+category, within 100 lines), got %d", len(result))
	}
}

func TestDeduplicate_HybridTier3_SameFileCloseLinesDifferentCategory(t *testing.T) {
	// Same file, close lines, but different categories.
	// Falls back to standard Jaccard threshold of 0.4.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "bug",
			Summary: "missing timeout on scan operation", Role: "solver"},
		{File: "a.go", Line: 12, Risk: "critical", Category: "security",
			Summary: "no timeout on scan operation", Role: "sentinel"},
	}
	result := Deduplicate(findings, 5)
	// Jaccard is ~0.6 (above 0.4), so they merge even with different categories.
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 3: close lines, high text similarity), got %d", len(result))
	}
}

func TestDeduplicate_HybridNoMatch_DifferentCategoryDistantLowSimilarity(t *testing.T) {
	// Same file, different categories, distant lines, low text similarity.
	// Should NOT merge — no tier matches.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "bug",
			Summary: "nil pointer dereference on empty input", Role: "solver"},
		{File: "a.go", Line: 100, Risk: "info", Category: "style",
			Summary: "consider renaming variable for clarity", Role: "editor"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (no match: different category, distant, low similarity), got %d", len(result))
	}
}

func TestDeduplicate_HybridNoMatch_SameCategoryDifferentFile(t *testing.T) {
	// Different files — should never merge regardless of other signals.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "design",
			Summary: "duplicate logic", Role: "architect"},
		{File: "b.go", Line: 10, Risk: "warning", Category: "design",
			Summary: "duplicate logic", Role: "editor"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (different files should never merge), got %d", len(result))
	}
}

func TestDeduplicate_HybridNoMatch_SameCategoryZeroSimilarity(t *testing.T) {
	// Same file, same category, nearby lines, but completely unrelated summaries.
	// Even tier 1's lenient 0.05 threshold should reject zero similarity.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "design",
			Summary: "function is too long", Role: "editor"},
		{File: "a.go", Line: 15, Risk: "info", Category: "design",
			Summary: "consider adding an interface", Role: "architect"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (zero text similarity despite structural match), got %d", len(result))
	}
}

func TestDeduplicate_HybridMergesRealWorldDuplicates(t *testing.T) {
	// Reproduces the actual duplicate findings from the Prism self-review
	// that motivated this feature. These 5 findings were all about the same
	// DRY issue but used different wording, causing them not to merge.
	findings := []Finding{
		{File: "report.go", Line: 961, Risk: "warning", Category: "design",
			Summary: "Code duplication: countRawSeverity and countDedupedSeverity are identical implementations", Role: "architect"},
		{File: "report.go", Line: 967, Risk: "warning", Category: "design",
			Summary: "Duplicate severity-counting logic across two nearly identical functions", Role: "editor"},
		{File: "report.go", Line: 960, Risk: "info", Category: "design",
			Summary: "Duplicated severity counting and sorting logic across three functions", Role: "know-it-all"},
		{File: "report.go", Line: 962, Risk: "info", Category: "design",
			Summary: "Duplicate severity counting logic violates DRY principle", Role: "solver"},
		{File: "report.go", Line: 973, Risk: "info", Category: "design",
			Summary: "Duplicated severity-counting and sorting logic across three locations", Role: "optimizer"},
	}
	result := Deduplicate(findings, 7)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding for the real-world DRY duplicates, got %d", len(result))
	}
	if result[0].VoteCount != 5 {
		t.Errorf("expected 5 votes, got %d", result[0].VoteCount)
	}
}

func TestDeduplicate_HybridEmptyCategoryFallsToTier3(t *testing.T) {
	// Empty categories should not qualify for tiers 1/2. These findings
	// are close enough and similar enough to merge via tier 3 (Jaccard >= 0.4).
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "", Summary: "duplicate code in function", Role: "a"},
		{File: "a.go", Line: 12, Risk: "warning", Category: "", Summary: "duplicate code in function", Role: "b"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 3: close lines, high similarity), got %d", len(result))
	}
}

func TestDeduplicate_HybridEmptyCategoryNoTier1(t *testing.T) {
	// Empty categories should NOT get tier 1's lenient threshold.
	// These findings have low text similarity and would only merge
	// via tier 1 — with empty categories they should stay separate.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "", Summary: "function is too long", Role: "editor"},
		{File: "a.go", Line: 15, Risk: "info", Category: "", Summary: "consider splitting logic", Role: "architect"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (empty categories skip tier 1), got %d", len(result))
	}
}

func TestBestSimilarity_SelectsHighestNotFirst(t *testing.T) {
	group := &DedupedFinding{
		Finding:        Finding{Summary: "error in code"},
		voterSummaries: []string{"error in code", "defect in implementation", "bug in procedure"},
	}
	candidate := "bug in method"
	best := bestSimilarity(group, candidate)
	// "bug in procedure" should match better than "error in code".
	worstMatch := jaccardSimilarity("error in code", candidate)
	if best <= worstMatch {
		t.Errorf("bestSimilarity should select highest match, got %.3f (worst was %.3f)", best, worstMatch)
	}
}

func TestDeduplicate_HybridTier2_RejectsDistantFindings(t *testing.T) {
	// Same file, same category, but lines 500 apart (beyond sameCategoryLineLimit=100).
	// Should NOT merge even with moderate text similarity.
	findings := []Finding{
		{File: "a.go", Line: 50, Risk: "info", Category: "design",
			Summary: "missing error handling in parse function", Role: "architect"},
		{File: "a.go", Line: 950, Risk: "info", Category: "design",
			Summary: "missing validation in format function", Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (tier 2 rejects distance > 100), got %d", len(result))
	}
}

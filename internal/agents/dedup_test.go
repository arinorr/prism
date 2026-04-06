package agents

import (
	"testing"
)

func TestDeduplicate_IdenticalFindings(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	result := Deduplicate(nil, 5)
	if len(result) != 0 {
		t.Fatalf("expected 0, got %d", len(result))
	}
}

func TestDeduplicate_SortOrder(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	s := jaccardSimilarity("missing timeout on scan", "missing timeout on scan")
	if s != 1.0 {
		t.Errorf("expected 1.0, got %f", s)
	}
}

func TestJaccardSimilarity_Similar(t *testing.T) {
	t.Parallel()
	s := jaccardSimilarity("missing timeout on scan operation", "no timeout on scan operation")
	// "missing" and "no" differ, rest overlap: 3/5 = 0.6.
	if s < jaccardThreshold {
		t.Errorf("expected above threshold, got %f", s)
	}
}

func TestJaccardSimilarity_Different(t *testing.T) {
	t.Parallel()
	s := jaccardSimilarity("missing timeout", "unused variable detected")
	if s >= jaccardThreshold {
		t.Errorf("expected below threshold for different summaries, got %f", s)
	}
}

func TestDeduplicate_PreservesAgentDetails(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	s := jaccardSimilarity("", "")
	if s != 1.0 {
		t.Errorf("expected 1.0 for both empty, got %f", s)
	}
}

// Hybrid dedup tests.

func TestDeduplicate_HybridTier1_SameFileCategoryNearbyLines(t *testing.T) {
	t.Parallel()
	// Same file, same category, lines within wideLineThreshold (20).
	// Summaries use different wording but prefix stemming creates enough
	// overlap to exceed tier 1's 0.065 threshold.
	s1 := "Code duplication: countRawSeverity and countDedupedSeverity are identical implementations"
	s2 := "Duplicated severity counting and sorting logic across three functions"

	// Verify the similarity is in the expected tier 1 range.
	sim := jaccardSimilarity(s1, s2)
	if sim < jaccardThresholdNearby {
		t.Fatalf("expected similarity >= %.2f (tier 1 threshold), got %.3f", jaccardThresholdNearby, sim)
	}
	if sim >= jaccardThreshold {
		t.Fatalf("expected similarity < %.2f (would match any tier), got %.3f", jaccardThreshold, sim)
	}

	findings := []Finding{
		{File: "report.go", Line: 961, Risk: "warning", Category: "design", Summary: s1, Role: "architect"},
		{File: "report.go", Line: 967, Risk: "warning", Category: "design", Summary: s2, Role: "editor"},
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
	t.Parallel()
	// Same file, same category, lines beyond wideLineThreshold (20) but
	// within sameCategoryLineLimit (100). Tier 2 threshold of 0.15 applies.
	s1 := "Duplicated severity counting logic across functions"
	s2 := "Duplicate counting and sorting logic across three locations"

	sim := jaccardSimilarity(s1, s2)
	if sim < jaccardThresholdSameCategory {
		t.Fatalf("expected similarity >= %.2f (tier 2 threshold), got %.3f", jaccardThresholdSameCategory, sim)
	}

	findings := []Finding{
		{File: "report.go", Line: 10, Risk: "info", Category: "design", Summary: s1, Role: "architect"},
		{File: "report.go", Line: 80, Risk: "info", Category: "design", Summary: s2, Role: "solver"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 2: same file+category, within 100 lines), got %d", len(result))
	}
}

func TestDeduplicate_HybridTier2_RejectsBelowThreshold(t *testing.T) {
	t.Parallel()
	// Same file, same category, distance 25 (past tier 1's 20-line limit).
	// Similarity is between tier 1 (0.065) and tier 2 (0.15) thresholds.
	// Should NOT merge — too low for tier 2, too far for tier 1.
	s1 := "duplicate logic in counting function"
	s2 := "redundant logic in formatting utility"

	sim := jaccardSimilarity(s1, s2)
	if sim >= jaccardThresholdSameCategory {
		t.Skipf("test data similarity %.3f is above tier 2 threshold, need different test data", sim)
	}
	if sim < jaccardThresholdNearby {
		t.Skipf("test data similarity %.3f is below tier 1 threshold, need different test data", sim)
	}

	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "info", Category: "design", Summary: s1, Role: "a"},
		{File: "a.go", Line: 35, Risk: "info", Category: "design", Summary: s2, Role: "b"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (similarity between tier 1 and tier 2 thresholds, distance > 20), got %d", len(result))
	}
}

func TestDeduplicate_HybridTier3_SameFileCloseLinesDifferentCategory(t *testing.T) {
	t.Parallel()
	// Same file, close lines, but different categories.
	// Falls back to standard Jaccard threshold of 0.4.
	s1 := "missing timeout on scan operation"
	s2 := "no timeout on scan operation"

	sim := jaccardSimilarity(s1, s2)
	if sim < jaccardThreshold {
		t.Fatalf("expected similarity >= %.2f (tier 3 threshold), got %.3f", jaccardThreshold, sim)
	}

	findings := []Finding{
		{File: "a.go", Line: 10, Risk: "warning", Category: "bug", Summary: s1, Role: "solver"},
		{File: "a.go", Line: 12, Risk: "critical", Category: "security", Summary: s2, Role: "sentinel"},
	}
	result := Deduplicate(findings, 5)
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped finding (tier 3: close lines, high text similarity), got %d", len(result))
	}
}

func TestDeduplicate_HybridNoMatch_DifferentCategoryDistantLowSimilarity(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	summaries := []string{"error in code", "defect in implementation", "bug in procedure"}
	tokens := make([]map[string]bool, len(summaries))
	for i, s := range summaries {
		tokens[i] = tokenize(s)
	}
	group := &DedupedFinding{
		Finding:     Finding{Summary: "error in code"},
		voterTokens: tokens,
	}
	candidateTokens := tokenize("bug in method")
	best := bestSimilarity(group, candidateTokens)
	// "bug in procedure" should be the best match, not "error in code" (first).
	expected := jaccardFromTokens(tokens[2], candidateTokens)
	firstMatch := jaccardFromTokens(tokens[0], candidateTokens)
	if best != expected {
		t.Errorf("bestSimilarity should return %.3f (best match), got %.3f", expected, best)
	}
	if best <= firstMatch {
		t.Errorf("best match (%.3f) should be better than first summary match (%.3f)", best, firstMatch)
	}
}

func TestBestSimilarity_ZeroValueWorks(t *testing.T) {
	t.Parallel()
	// A DedupedFinding constructed without voterTokens should still work —
	// bestSimilarity derives tokens from the embedded Finding's Summary.
	group := &DedupedFinding{
		Finding: Finding{Summary: "bug in procedure"},
	}
	candidateTokens := tokenize("bug in method")
	best := bestSimilarity(group, candidateTokens)
	expected := jaccardFromTokens(tokenize("bug in procedure"), candidateTokens)
	if best != expected {
		t.Errorf("zero-value group should match via Summary, got %.3f want %.3f", best, expected)
	}
	if best == 0.0 {
		t.Error("zero-value group should produce non-zero similarity for related summaries")
	}
}

func TestDeduplicate_HybridTier2_RejectsDistantFindings(t *testing.T) {
	t.Parallel()
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

func TestDeduplicate_HybridTier2_BoundaryAtLimit(t *testing.T) {
	t.Parallel()
	// Lines exactly at sameCategoryLineLimit (100) should still merge via tier 2.
	// Lines at 101 should NOT merge (falls to tier 3 which needs higher similarity).
	findings100 := []Finding{
		{File: "a.go", Line: 10, Risk: "info", Category: "design",
			Summary: "duplicated counting logic across functions", Role: "a"},
		{File: "a.go", Line: 110, Risk: "info", Category: "design",
			Summary: "duplicate counting and sorting logic", Role: "b"},
	}
	result100 := Deduplicate(findings100, 5)
	if len(result100) != 1 {
		t.Errorf("expected 1 finding at distance=100 (tier 2 boundary), got %d", len(result100))
	}

	findings101 := []Finding{
		{File: "a.go", Line: 10, Risk: "info", Category: "design",
			Summary: "duplicated counting logic across functions", Role: "a"},
		{File: "a.go", Line: 111, Risk: "info", Category: "design",
			Summary: "duplicate counting and sorting logic", Role: "b"},
	}
	result101 := Deduplicate(findings101, 5)
	if len(result101) != 2 {
		t.Errorf("expected 2 findings at distance=101 (beyond tier 2), got %d", len(result101))
	}
}

func TestDeduplicate_HybridOrderIndependence(t *testing.T) {
	t.Parallel()
	// The same findings in different order should produce the same group count.
	// This validates that bestSimilarity's multi-summary matching prevents
	// greedy-ordering effects.
	base := []Finding{
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
	forwardCount := len(Deduplicate(base, 7))

	reversed := make([]Finding, len(base))
	for i, f := range base {
		reversed[len(base)-1-i] = f
	}
	reversedCount := len(Deduplicate(reversed, 7))

	if forwardCount != reversedCount {
		t.Errorf("order sensitivity: forward=%d groups, reversed=%d groups", forwardCount, reversedCount)
	}
}

// Edge cases.

func TestDeduplicate_SameLineSameFile(t *testing.T) {
	t.Parallel()
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
	deduped := Deduplicate(findings, 2)
	if len(deduped) == 0 {
		t.Error("expected at least 1 deduped finding even with empty summaries")
	}
}

func TestDeduplicate_SingleFindingWithTotalAgents(t *testing.T) {
	t.Parallel()
	deduped := Deduplicate([]Finding{
		{File: "a.go", Line: 1, Risk: RiskInfo, Category: "style", Scope: ScopeChanged, Summary: "test", Detail: "d", Role: "editor"},
	}, 7)
	if len(deduped) != 1 {
		t.Errorf("expected 1, got %d", len(deduped))
	}
	if deduped[0].VoteCount != 1 {
		t.Errorf("expected 1 vote, got %d", deduped[0].VoteCount)
	}
	if deduped[0].TotalAgents != 7 {
		t.Errorf("expected 7 total agents, got %d", deduped[0].TotalAgents)
	}
}

func TestDeduplicate_DifferentFilesSameIssue(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Missing nil check", Detail: "d", Role: "sentinel"},
		{File: "b.go", Line: 10, Risk: RiskWarning, Category: "bug", Scope: ScopeChanged, Summary: "Missing nil check", Detail: "d", Role: "solver"},
	}
	deduped := Deduplicate(findings, 2)
	if len(deduped) != 2 {
		t.Errorf("different files should not merge, got %d", len(deduped))
	}
}

func TestDeduplicate_NilAndEmptySlice(t *testing.T) {
	t.Parallel()
	if len(Deduplicate(nil, 7)) != 0 {
		t.Error("nil findings should produce empty deduped")
	}
	if len(Deduplicate([]Finding{}, 7)) != 0 {
		t.Error("empty findings should produce empty deduped")
	}
}

// Dual-score fields.

func TestDeduplicate_AgentDetailPreservesRisk(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskCritical, Category: "bug", Summary: "nil ptr", Detail: "d", Role: "sentinel"},
		{File: "a.go", Line: 10, Risk: RiskInfo, Category: "bug", Summary: "nil pointer", Detail: "d", Role: "editor"},
	}
	deduped := Deduplicate(findings, 7)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduped, got %d", len(deduped))
	}
	if len(deduped[0].AgentDetails) != 2 {
		t.Fatalf("expected 2 agent details, got %d", len(deduped[0].AgentDetails))
	}
	// Each detail should preserve the original voter's risk.
	risks := map[Risk]bool{}
	for _, ad := range deduped[0].AgentDetails {
		risks[ad.Risk] = true
	}
	if !risks[RiskCritical] || !risks[RiskInfo] {
		t.Errorf("agent details should preserve both critical and info, got %v", risks)
	}
}

func TestDeduplicate_ConsensusMethod(t *testing.T) {
	t.Parallel()
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Summary: "issue", Detail: "d", Role: "a"},
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Summary: "issue", Detail: "d", Role: "b"},
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Summary: "issue", Detail: "d", Role: "c"},
	}
	deduped := Deduplicate(findings, 7)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduped, got %d", len(deduped))
	}
	consensus := deduped[0].Consensus()
	expected := 3.0 / 7.0
	if consensus < expected-0.01 || consensus > expected+0.01 {
		t.Errorf("expected consensus %.3f, got %.3f", expected, consensus)
	}
}

func TestDeduplicate_CompositeSeverityMethod(t *testing.T) {
	t.Parallel()
	// 2 criticals + 1 info → composite should be > 2.0 (gravity amplifies criticals).
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskCritical, Category: "bug", Summary: "nil ptr deref", Detail: "d", Role: "sentinel"},
		{File: "a.go", Line: 10, Risk: RiskCritical, Category: "bug", Summary: "nil pointer", Detail: "d", Role: "solver"},
		{File: "a.go", Line: 10, Risk: RiskInfo, Category: "bug", Summary: "nil pointer issue", Detail: "d", Role: "editor"},
	}
	deduped := Deduplicate(findings, 7)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduped, got %d", len(deduped))
	}
	cs := deduped[0].CompositeSeverity()
	if cs <= 2.0 {
		t.Errorf("2 criticals + 1 info with gravity should produce composite > 2.0, got %.2f", cs)
	}
}

func TestDeduplicate_RiskDerivedFromComposite(t *testing.T) {
	t.Parallel()
	// All agents say warning → composite around 2.0 → Risk should be warning.
	findings := []Finding{
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Summary: "issue here", Detail: "d", Role: "a"},
		{File: "a.go", Line: 10, Risk: RiskWarning, Category: "bug", Summary: "issue here", Detail: "d", Role: "b"},
	}
	deduped := Deduplicate(findings, 7)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduped, got %d", len(deduped))
	}
	if deduped[0].Risk != RiskWarning {
		t.Errorf("unanimous warning should derive Risk=warning, got %q", deduped[0].Risk)
	}
}

func TestDisagreementSpread_High(t *testing.T) {
	t.Parallel()
	details := []AgentDetail{
		{Risk: RiskCritical},
		{Risk: RiskInfo},
	}
	if spread := DisagreementSpread(details); spread != 2 {
		t.Errorf("critical vs info spread should be 2, got %d", spread)
	}
}

func TestDisagreementSpread_Low(t *testing.T) {
	t.Parallel()
	details := []AgentDetail{
		{Risk: RiskWarning},
		{Risk: RiskWarning},
		{Risk: RiskWarning},
	}
	if spread := DisagreementSpread(details); spread != 0 {
		t.Errorf("all warning spread should be 0, got %d", spread)
	}
}

func TestDisagreementSpread_SingleAgent(t *testing.T) {
	t.Parallel()
	if spread := DisagreementSpread([]AgentDetail{{Risk: RiskCritical}}); spread != 0 {
		t.Errorf("single agent spread should be 0, got %d", spread)
	}
}

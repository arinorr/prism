package agents

import "testing"

// Helper to build a DedupedFinding with proper AgentDetails for score tests.
func dedupedFinding(risk Risk, scope string, voteCount, totalAgents int, voterRisks []Risk, voterRoles []string, category string) DedupedFinding {
	details := make([]AgentDetail, len(voterRisks))
	voters := make([]string, len(voterRoles))
	for i := range voterRisks {
		role := "agent"
		if i < len(voterRoles) {
			role = voterRoles[i]
		}
		details[i] = AgentDetail{Role: role, Risk: voterRisks[i]}
		voters[i] = role
	}
	return DedupedFinding{
		Finding:      Finding{Risk: risk, Scope: scope, Category: category},
		VoteCount:    voteCount,
		TotalAgents:  totalAgents,
		Voters:       voters,
		AgentDetails: details,
	}
}

// unanimousFinding builds a finding where all voters agree on the same risk.
func unanimousFinding(risk Risk, scope string, votes, total int, category string) DedupedFinding {
	risks := make([]Risk, votes)
	roles := make([]string, votes)
	for i := range risks {
		risks[i] = risk
		roles[i] = "agent"
	}
	return dedupedFinding(risk, scope, votes, total, risks, roles, category)
}

func TestComputeHealthScore_Perfect(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 {
		t.Errorf("expected 100, got %d", score.Score)
	}
	if score.Grade != "A+" {
		t.Errorf("expected A+, got %q", score.Grade)
	}
	if score.Verdict != VerdictApprove {
		t.Errorf("expected approve, got %q", score.Verdict)
	}
}

func TestComputeHealthScore_EmptyFindings(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore([]DedupedFinding{})
	if score.Score != 100 {
		t.Errorf("expected 100, got %d", score.Score)
	}
}

func TestComputeHealthScore_OneCriticalChanged(t *testing.T) {
	t.Parallel()
	// 7/7 unanimous critical in changed scope.
	// basePenalty=8, CompositeSeverity=3.0 (all critical), Consensus=1.0
	// deduction = 8 * 3.0 * 1.0 = 24 → score 76
	f := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategorySecurity)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 76 {
		t.Errorf("expected 76, got %d", score.Score)
	}
	if score.Grade != "B" {
		t.Errorf("expected B, got %q", score.Grade)
	}
	if score.Verdict != VerdictSuggestions {
		t.Errorf("expected %q, got %q", VerdictSuggestions, score.Verdict)
	}
}

func TestComputeHealthScore_WeightedByVotes(t *testing.T) {
	t.Parallel()
	// 1/7 critical in changed scope.
	// basePenalty=8, CompositeSeverity=3.0, Consensus=1/7≈0.143
	// deduction = 8 * 3.0 * 0.143 ≈ 3.43 → score 97
	f := unanimousFinding(RiskCritical, ScopeChanged, 1, 7, CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 97 {
		t.Errorf("expected 97 (low-vote critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_ExistingIssuesLessImpact(t *testing.T) {
	t.Parallel()
	// 7/7 critical in existing scope.
	// basePenalty=2, CompositeSeverity=3.0, Consensus=1.0
	// deduction = 2 * 3.0 * 1.0 = 6 → score 94
	f := unanimousFinding(RiskCritical, ScopeExisting, 7, 7, CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 94 {
		t.Errorf("expected 94 (existing critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_CodebaseMinimalImpact(t *testing.T) {
	t.Parallel()
	// 3/7 warning in codebase scope.
	// basePenalty=0.5, CompositeSeverity=2.0, Consensus=3/7≈0.429
	// deduction = 0.5 * 2.0 * 0.429 ≈ 0.43 → score 100
	f := unanimousFinding(RiskWarning, ScopeCodebase, 3, 7, CategoryDesign)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 100 {
		t.Errorf("expected 100 (codebase is minimal), got %d", score.Score)
	}
}

func TestComputeHealthScore_Floor(t *testing.T) {
	t.Parallel()
	// 10 unanimous 5/5 criticals in changed scope.
	// Each: 8 * 3.0 * 1.0 = 24. Total = 240. Floored to 0.
	findings := make([]DedupedFinding, 10)
	for i := range findings {
		findings[i] = unanimousFinding(RiskCritical, ScopeChanged, 5, 5, CategoryBug)
	}
	score := ComputeHealthScore(findings)
	if score.Score != 0 {
		t.Errorf("expected 0 (floor), got %d", score.Score)
	}
	if score.Grade != "F" {
		t.Errorf("expected F, got %q", score.Grade)
	}
}

func TestComputeHealthScore_GradeBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score int
		grade string
	}{
		{100, "A+"}, {95, "A+"}, {94, "A"}, {90, "A"},
		{89, "B+"}, {80, "B+"}, {79, "B"}, {70, "B"},
		{69, "C"}, {60, "C"}, {59, "D"}, {40, "D"},
		{39, "F"}, {0, "F"},
	}
	for _, tt := range tests {
		got := scoreToGrade(tt.score)
		if got != tt.grade {
			t.Errorf("scoreToGrade(%d) = %q, want %q", tt.score, got, tt.grade)
		}
	}
}

func TestComputeHealthScore_MixedFindings(t *testing.T) {
	t.Parallel()
	findings := []DedupedFinding{
		// 5/7 unanimous critical changed: 8 * 3.0 * 5/7 ≈ 17.14
		unanimousFinding(RiskCritical, ScopeChanged, 5, 7, CategorySecurity),
		// 3/7 unanimous warning changed: 8 * 2.0 * 3/7 ≈ 6.86
		unanimousFinding(RiskWarning, ScopeChanged, 3, 7, CategoryDesign),
		// 1/7 unanimous info changed: 8 * 1.0 * 1/7 ≈ 1.14
		unanimousFinding(RiskInfo, ScopeChanged, 1, 7, CategoryStyle),
		// 2/7 unanimous critical existing: 2 * 3.0 * 2/7 ≈ 1.71
		unanimousFinding(RiskCritical, ScopeExisting, 2, 7, CategoryBug),
	}
	// Total deductions: 17.14 + 6.86 + 1.14 + 1.71 ≈ 26.86 → score 73
	score := ComputeHealthScore(findings)
	if score.Score != 73 {
		t.Errorf("expected 73, got %d", score.Score)
	}
}

// Edge cases.

func TestHealthScore_NoFindings(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 || score.Grade != "A+" {
		t.Errorf("no findings: expected 100/A+, got %d/%s", score.Score, score.Grade)
	}
}

func TestHealthScore_ManyHighConsensusFindings(t *testing.T) {
	t.Parallel()
	// 5 unanimous 7/7 criticals in changed scope.
	// Each: 8 * 3.0 * 1.0 = 24. Total = 120. Score = 0.
	findings := make([]DedupedFinding, 5)
	for i := range findings {
		findings[i] = unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug)
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
	low := ComputeHealthScore([]DedupedFinding{
		unanimousFinding(RiskCritical, ScopeChanged, 1, 7, CategoryBug),
	})
	high := ComputeHealthScore([]DedupedFinding{
		unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug),
	})
	if low.Score <= high.Score {
		t.Errorf("low consensus (%d) should score higher than high consensus (%d)", low.Score, high.Score)
	}
}

// Domain authority and composite severity scoring.

func TestComputeHealthScore_DomainExpertAmplifies(t *testing.T) {
	t.Parallel()
	// When agents disagree, domain authority matters: the expert's opinion
	// carries more weight in the composite. Sentinel (security expert) saying
	// critical + 2 non-experts saying info → higher composite severity than
	// Editor (non-expert for security) saying critical + 2 non-experts saying info.
	expertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"sentinel", "editor", "test-engineer"},
		CategorySecurity)
	nonExpertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"editor", "sentinel", "test-engineer"}, // editor is not a security expert
		CategorySecurity)

	// Domain expert (sentinel) saying critical on security should produce
	// higher CompositeSeverity than non-expert saying critical.
	expertCS := expertCritical.CompositeSeverity()
	nonExpertCS := nonExpertCritical.CompositeSeverity()
	if expertCS <= nonExpertCS {
		t.Errorf("expert critical (%.2f) should have higher composite than non-expert critical (%.2f)",
			expertCS, nonExpertCS)
	}
}

func TestComputeHealthScore_DisagreedCriticalLessHarsh(t *testing.T) {
	t.Parallel()
	// Unanimous critical: all 7 say critical.
	unanimous := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug)

	// Disagreed: 2 say critical, 5 say info. CompositeSeverity will be lower.
	disagreed := dedupedFinding(RiskWarning, ScopeChanged, 7, 7,
		[]Risk{RiskCritical, RiskCritical, RiskInfo, RiskInfo, RiskInfo, RiskInfo, RiskInfo},
		[]string{"sentinel", "solver", "editor", "know-it-all", "architect", "optimizer", "test-engineer"},
		CategoryBug)

	unanimousScore := ComputeHealthScore([]DedupedFinding{unanimous})
	disagreedScore := ComputeHealthScore([]DedupedFinding{disagreed})

	// Disagreed should be less harsh (higher score).
	if disagreedScore.Score <= unanimousScore.Score {
		t.Errorf("disagreed (%d) should score higher than unanimous (%d)",
			disagreedScore.Score, unanimousScore.Score)
	}
}

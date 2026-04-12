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
	if score.Grade != "A" {
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
	// basePenalty=3, CompositeSeverity=3.0, Consensus=1.0
	// deduction = 3 * 3.0 * 1.0 = 9 → score 91
	f := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategorySecurity)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 91 {
		t.Errorf("expected 91, got %d", score.Score)
	}
	if score.Grade != "A-" {
		t.Errorf("expected A-, got %q", score.Grade)
	}
}

func TestComputeHealthScore_WeightedByVotes(t *testing.T) {
	t.Parallel()
	// 1/7 critical in changed scope.
	// basePenalty=3, CompositeSeverity=3.0, Consensus=1/7≈0.143
	// deduction = 3 * 3.0 * 0.143 ≈ 1.29 → score 99
	f := unanimousFinding(RiskCritical, ScopeChanged, 1, 7, CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 99 {
		t.Errorf("expected 99 (low-vote critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_ExistingIssuesLessImpact(t *testing.T) {
	t.Parallel()
	// 7/7 critical in existing scope.
	// basePenalty=1, CompositeSeverity=3.0, Consensus=1.0
	// deduction = 1 * 3.0 * 1.0 = 3 → score 97
	f := unanimousFinding(RiskCritical, ScopeExisting, 7, 7, CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 97 {
		t.Errorf("expected 97 (existing critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_CodebaseMinimalImpact(t *testing.T) {
	t.Parallel()
	// 3/7 warning in codebase scope.
	// basePenalty=0.25, CompositeSeverity=2.0, Consensus=3/7≈0.429
	// deduction = 0.25 * 2.0 * 0.429 ≈ 0.21 → score 100
	f := unanimousFinding(RiskWarning, ScopeCodebase, 3, 7, CategoryDesign)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score != 100 {
		t.Errorf("expected 100 (codebase is minimal), got %d", score.Score)
	}
}

func TestComputeHealthScore_Floor(t *testing.T) {
	t.Parallel()
	// 10 unanimous 5/5 criticals in changed scope.
	// Each: 3 * 3.0 * 1.0 = 9. Total = 90. Score = 10.
	findings := make([]DedupedFinding, 10)
	for i := range findings {
		findings[i] = unanimousFinding(RiskCritical, ScopeChanged, 5, 5, CategoryBug)
	}
	score := ComputeHealthScore(findings)
	if score.Score != 10 {
		t.Errorf("expected 10, got %d", score.Score)
	}
}

func TestComputeHealthScore_GradeBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score int
		grade string
	}{
		{100, "A"}, {93, "A"}, {92, "A-"}, {90, "A-"},
		{89, "B+"}, {87, "B+"}, {86, "B"}, {83, "B"},
		{82, "B-"}, {80, "B-"}, {79, "C+"}, {77, "C+"},
		{76, "C"}, {73, "C"}, {72, "C-"}, {70, "C-"},
		{69, "D+"}, {67, "D+"}, {66, "D"}, {63, "D"},
		{62, "D-"}, {60, "D-"}, {59, "F"}, {0, "F"},
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
		// 5/7 unanimous critical changed: 3 * 3.0 * 5/7 ≈ 6.43
		unanimousFinding(RiskCritical, ScopeChanged, 5, 7, CategorySecurity),
		// 3/7 unanimous warning changed: 3 * 2.0 * 3/7 ≈ 2.57
		unanimousFinding(RiskWarning, ScopeChanged, 3, 7, CategoryDesign),
		// 1/7 unanimous info changed: 3 * 1.0 * 1/7 ≈ 0.43
		unanimousFinding(RiskInfo, ScopeChanged, 1, 7, CategoryStyle),
		// 2/7 unanimous critical existing: 1 * 3.0 * 2/7 ≈ 0.86
		unanimousFinding(RiskCritical, ScopeExisting, 2, 7, CategoryBug),
	}
	// Total: 6.43 + 2.57 + 0.43 + 0.86 ≈ 10.29 → score 90
	score := ComputeHealthScore(findings)
	if score.Score != 90 {
		t.Errorf("expected 90, got %d", score.Score)
	}
}

// Edge cases.

func TestHealthScore_NoFindings(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 || score.Grade != "A" {
		t.Errorf("no findings: expected 100/A, got %d/%s", score.Score, score.Grade)
	}
}

func TestHealthScore_ManyHighConsensusFindings(t *testing.T) {
	t.Parallel()
	// 5 unanimous 7/7 criticals in changed scope.
	// Each: 3 * 3.0 * 1.0 = 9. Total = 45. Score = 55.
	findings := make([]DedupedFinding, 5)
	for i := range findings {
		findings[i] = unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug)
	}
	score := ComputeHealthScore(findings)
	if score.Score != 55 {
		t.Errorf("5 unanimous criticals: expected 55, got %d", score.Score)
	}
	if score.Grade != "F" {
		t.Errorf("expected grade F, got %q", score.Grade)
	}
	if score.Verdict != VerdictDiscuss {
		t.Errorf("expected verdict %q, got %q", VerdictRequestChanges, score.Verdict)
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
	// When agents disagree, domain authority matters.
	expertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"sentinel", "editor", "test-engineer"},
		CategorySecurity)
	nonExpertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"editor", "sentinel", "test-engineer"},
		CategorySecurity)

	expertCS := expertCritical.CompositeSeverity()
	nonExpertCS := nonExpertCritical.CompositeSeverity()
	if expertCS <= nonExpertCS {
		t.Errorf("expert critical (%.2f) should have higher composite than non-expert critical (%.2f)",
			expertCS, nonExpertCS)
	}
}

func TestComputeHealthScore_DisagreedCriticalLessHarsh(t *testing.T) {
	t.Parallel()
	unanimous := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug)
	disagreed := dedupedFinding(RiskWarning, ScopeChanged, 7, 7,
		[]Risk{RiskCritical, RiskCritical, RiskInfo, RiskInfo, RiskInfo, RiskInfo, RiskInfo},
		[]string{"sentinel", "solver", "editor", "know-it-all", "architect", "optimizer", "test-engineer"},
		CategoryBug)

	unanimousScore := ComputeHealthScore([]DedupedFinding{unanimous})
	disagreedScore := ComputeHealthScore([]DedupedFinding{disagreed})

	if disagreedScore.Score <= unanimousScore.Score {
		t.Errorf("disagreed (%d) should score higher than unanimous (%d)",
			disagreedScore.Score, unanimousScore.Score)
	}
}

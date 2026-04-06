package agents

import "testing"

func TestComputeHealthScore_Perfect(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 {
		t.Errorf("expected 100, got %d", score.Score)
	}
	if score.Grade != "A+" {
		t.Errorf("expected A+, got %q", score.Grade)
	}
	if score.Verdict != "approve" {
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
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: RiskCritical, Scope: ScopeChanged},
			VoteCount:   7,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	// Full weight critical = -20, so 80.
	if score.Score != 80 {
		t.Errorf("expected 80, got %d", score.Score)
	}
	if score.Grade != "B+" {
		t.Errorf("expected B+, got %q", score.Grade)
	}
	if score.Verdict != "approve with suggestions" {
		t.Errorf("expected 'approve with suggestions', got %q", score.Verdict)
	}
}

func TestComputeHealthScore_WeightedByVotes(t *testing.T) {
	t.Parallel()
	// 1/7 vote on a critical = -20 * (1/7) ≈ -2.86, score ≈ 97.
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: RiskCritical, Scope: ScopeChanged},
			VoteCount:   1,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	if score.Score != 97 {
		t.Errorf("expected 97 (low-vote critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_ExistingIssuesLessImpact(t *testing.T) {
	t.Parallel()
	// Existing critical = -5 (full weight).
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: RiskCritical, Scope: ScopeExisting},
			VoteCount:   7,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	if score.Score != 95 {
		t.Errorf("expected 95 (existing critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_CodebaseMinimalImpact(t *testing.T) {
	t.Parallel()
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: RiskWarning, Scope: ScopeCodebase},
			VoteCount:   3,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	// Codebase = -1 * (3/7) ≈ -0.43, score ≈ 100.
	if score.Score != 100 {
		t.Errorf("expected 100 (codebase is minimal), got %d", score.Score)
	}
}

func TestComputeHealthScore_Floor(t *testing.T) {
	t.Parallel()
	// 10 full-weight criticals in changed scope = -200, floored to 0.
	findings := make([]DedupedFinding, 10)
	for i := range findings {
		findings[i] = DedupedFinding{
			Finding:     Finding{Risk: RiskCritical, Scope: ScopeChanged},
			VoteCount:   7,
			TotalAgents: 7,
		}
	}
	score := ComputeHealthScore(findings)
	if score.Score != 0 {
		t.Errorf("expected 0 (floor), got %d", score.Score)
	}
	if score.Grade != "F" {
		t.Errorf("expected F, got %q", score.Grade)
	}
	if score.Verdict != VerdictDiscuss {
		t.Errorf("expected %q, got %q", VerdictDiscuss, score.Verdict)
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
		{Finding: Finding{Risk: RiskCritical, Scope: ScopeChanged}, VoteCount: 5, TotalAgents: 7},  // -20 * 5/7 ≈ -14.3
		{Finding: Finding{Risk: RiskWarning, Scope: ScopeChanged}, VoteCount: 3, TotalAgents: 7},   // -8 * 3/7 ≈ -3.4
		{Finding: Finding{Risk: RiskInfo, Scope: ScopeChanged}, VoteCount: 1, TotalAgents: 7},      // -2 * 1/7 ≈ -0.3
		{Finding: Finding{Risk: RiskCritical, Scope: ScopeExisting}, VoteCount: 2, TotalAgents: 7}, // -5 * 2/7 ≈ -1.4
	}
	score := ComputeHealthScore(findings)
	// 100 - 14.3 - 3.4 - 0.3 - 1.4 ≈ 80.6 → 81
	if score.Score != 81 {
		t.Errorf("expected ~81, got %d", score.Score)
	}
}

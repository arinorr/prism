package agents

import "testing"

func TestComputeHealthScore_Perfect(t *testing.T) {
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
	score := ComputeHealthScore([]DedupedFinding{})
	if score.Score != 100 {
		t.Errorf("expected 100, got %d", score.Score)
	}
}

func TestComputeHealthScore_OneCriticalChanged(t *testing.T) {
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: SeverityCritical, Scope: ScopeChanged},
			VoteCount:   7,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	// basePenalty=8 × severity=3.0 × confidence=1.0 = 24, score = 76.
	if score.Score != 76 {
		t.Errorf("expected 76, got %d", score.Score)
	}
	if score.Grade != "B" {
		t.Errorf("expected B, got %q", score.Grade)
	}
	if score.Verdict != "approve with suggestions" {
		t.Errorf("expected 'approve with suggestions', got %q", score.Verdict)
	}
}

func TestComputeHealthScore_WeightedByVotes(t *testing.T) {
	// 1/7 vote on a critical = -20 * (1/7) ≈ -2.86, score ≈ 97.
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: SeverityCritical, Scope: ScopeChanged},
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
	// basePenalty=2 × severity=3.0 × confidence=1.0 = 6, score = 94.
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: SeverityCritical, Scope: ScopeExisting},
			VoteCount:   7,
			TotalAgents: 7,
		},
	}
	score := ComputeHealthScore(findings)
	if score.Score != 94 {
		t.Errorf("expected 94 (existing critical), got %d", score.Score)
	}
}

func TestComputeHealthScore_CodebaseMinimalImpact(t *testing.T) {
	findings := []DedupedFinding{
		{
			Finding:     Finding{Risk: SeverityWarning, Scope: ScopeCodebase},
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
	// 10 full-weight criticals in changed scope = -200, floored to 0.
	findings := make([]DedupedFinding, 10)
	for i := range findings {
		findings[i] = DedupedFinding{
			Finding:     Finding{Risk: SeverityCritical, Scope: ScopeChanged},
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
	if score.Verdict != "needs discussion" {
		t.Errorf("expected 'needs discussion', got %q", score.Verdict)
	}
}

func TestComputeHealthScore_GradeBoundaries(t *testing.T) {
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
	findings := []DedupedFinding{
		{Finding: Finding{Risk: SeverityCritical, Scope: ScopeChanged}, VoteCount: 5, TotalAgents: 7},  // 8 * 3.0 * 5/7 = 17.14
		{Finding: Finding{Risk: SeverityWarning, Scope: ScopeChanged}, VoteCount: 3, TotalAgents: 7},   // 8 * 2.0 * 3/7 = 6.86
		{Finding: Finding{Risk: SeverityInfo, Scope: ScopeChanged}, VoteCount: 1, TotalAgents: 7},      // 8 * 1.0 * 1/7 = 1.14
		{Finding: Finding{Risk: SeverityCritical, Scope: ScopeExisting}, VoteCount: 2, TotalAgents: 7}, // 2 * 3.0 * 2/7 = 1.71
	}
	score := ComputeHealthScore(findings)
	// 100 - 17.14 - 6.86 - 1.14 - 1.71 ≈ 73.15 → 73
	if score.Score != 73 {
		t.Errorf("expected 73, got %d", score.Score)
	}
}

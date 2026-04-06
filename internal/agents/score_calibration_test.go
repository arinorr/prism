package agents

import "testing"

// Calibration tests verify score RANGES for realistic scenarios.
// These catch formula regressions without being brittle to exact values.

func TestCalibration_CleanPR(t *testing.T) {
	t.Parallel()
	score := ComputeHealthScore(nil)
	if score.Score != 100 {
		t.Errorf("clean PR: expected 100, got %d", score.Score)
	}
	if score.Verdict != VerdictApprove {
		t.Errorf("clean PR: expected approve, got %q", score.Verdict)
	}
}

func TestCalibration_MinorStyleIssues(t *testing.T) {
	t.Parallel()
	// 3 info findings, each from 1/7 agents.
	findings := make([]DedupedFinding, 3)
	for i := range findings {
		findings[i] = unanimousFinding(RiskInfo, ScopeChanged, 1, 7, CategoryStyle)
	}
	score := ComputeHealthScore(findings)
	if score.Score < 95 {
		t.Errorf("minor style issues: expected score >= 95, got %d", score.Score)
	}
	if score.Verdict != VerdictApprove {
		t.Errorf("minor style issues: expected approve, got %q", score.Verdict)
	}
}

func TestCalibration_UnanimousCritical(t *testing.T) {
	t.Parallel()
	// 1 finding, all 7 agents say critical.
	f := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategorySecurity)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score < 85 || score.Score > 95 {
		t.Errorf("unanimous critical: expected score 85-95, got %d", score.Score)
	}
}

func TestCalibration_DisagreedCritical(t *testing.T) {
	t.Parallel()
	// 2/7 say critical, 5/7 say info. Composite severity much lower.
	f := dedupedFinding(RiskWarning, ScopeChanged, 7, 7,
		[]Risk{RiskCritical, RiskCritical, RiskInfo, RiskInfo, RiskInfo, RiskInfo, RiskInfo},
		[]string{"sentinel", "solver", "editor", "know-it-all", "architect", "optimizer", "test-engineer"},
		CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score < 90 {
		t.Errorf("disagreed critical: expected score >= 90 (nuanced), got %d", score.Score)
	}
}

func TestCalibration_MultipleMixed(t *testing.T) {
	t.Parallel()
	findings := []DedupedFinding{
		unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategorySecurity), // heavy
		unanimousFinding(RiskWarning, ScopeChanged, 5, 7, CategoryDesign),    // moderate
		unanimousFinding(RiskWarning, ScopeChanged, 3, 7, CategoryBug),       // light
		unanimousFinding(RiskInfo, ScopeExisting, 2, 7, CategoryStyle),       // minimal
		unanimousFinding(RiskInfo, ScopeCodebase, 1, 7, CategoryTesting),     // negligible
	}
	score := ComputeHealthScore(findings)
	if score.Score < 75 || score.Score > 90 {
		t.Errorf("multiple mixed: expected score 75-90, got %d", score.Score)
	}
}

func TestCalibration_DomainExpertAmplification(t *testing.T) {
	t.Parallel()
	// Sentinel (security expert) says critical on security finding + 2 say info.
	expertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"sentinel", "editor", "test-engineer"},
		CategorySecurity)

	// Editor (NOT security expert) says critical on security finding + 2 say info.
	nonExpertCritical := dedupedFinding(RiskWarning, ScopeChanged, 3, 7,
		[]Risk{RiskCritical, RiskInfo, RiskInfo},
		[]string{"editor", "sentinel", "test-engineer"},
		CategorySecurity)

	expertScore := ComputeHealthScore([]DedupedFinding{expertCritical})
	nonExpertScore := ComputeHealthScore([]DedupedFinding{nonExpertCritical})

	if expertScore.Score >= nonExpertScore.Score {
		t.Errorf("domain expert should produce lower score (%d) than non-expert (%d)",
			expertScore.Score, nonExpertScore.Score)
	}
}

// Property tests.

func TestProperty_MonotonicitySeverity(t *testing.T) {
	t.Parallel()
	// Higher CompositeSeverity → larger deduction → lower score.
	low := unanimousFinding(RiskInfo, ScopeChanged, 7, 7, CategoryStyle)
	high := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategorySecurity)

	lowScore := ComputeHealthScore([]DedupedFinding{low})
	highScore := ComputeHealthScore([]DedupedFinding{high})

	if lowScore.Score <= highScore.Score {
		t.Errorf("info (%d) should score higher than critical (%d)", lowScore.Score, highScore.Score)
	}
}

func TestProperty_ConsensusAmplifies(t *testing.T) {
	t.Parallel()
	few := unanimousFinding(RiskCritical, ScopeChanged, 3, 7, CategoryBug)
	all := unanimousFinding(RiskCritical, ScopeChanged, 7, 7, CategoryBug)

	fewScore := ComputeHealthScore([]DedupedFinding{few})
	allScore := ComputeHealthScore([]DedupedFinding{all})

	if fewScore.Score <= allScore.Score {
		t.Errorf("3/7 (%d) should score higher than 7/7 (%d)", fewScore.Score, allScore.Score)
	}
}

func TestProperty_FloorPreservation(t *testing.T) {
	t.Parallel()
	// Even 1/7 critical vote should produce a meaningful deduction (not zero).
	f := unanimousFinding(RiskCritical, ScopeChanged, 1, 7, CategoryBug)
	score := ComputeHealthScore([]DedupedFinding{f})
	if score.Score >= 100 {
		t.Errorf("1/7 critical should still deduct something, got score %d", score.Score)
	}
}

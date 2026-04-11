// Package agents — score.go computes a PR health score from deduped findings.
package agents

import "fmt"

// Verdict constants returned by ComputeHealthScore.
const (
	VerdictApprove        = "approve"
	VerdictSuggestions    = "approve with suggestions"
	VerdictRequestChanges = "request changes"
	VerdictDiscuss        = "needs discussion"
)

// HealthScore summarizes the overall quality of a PR based on findings.
type HealthScore struct {
	Score       int    `json:"score"`       // 0-100
	Grade       string `json:"grade"`       // A+, A, B+, B, C, D, F
	Verdict     string `json:"verdict"`     // approve, approve with suggestions, request changes, needs discussion
	Description string `json:"description"` // human-readable one-liner
}

// ComputeHealthScore computes a 0-100 health score from deduped findings.
// Uses two independent signals:
//   - Consensus (0.0-1.0): what fraction of agents flagged this (gates the deduction)
//   - CompositeSeverity (1.0-3.0): weighted consensus on how bad it is (scales the deduction)
//
// Formula: deduction = basePenalty × compositeSeverity × consensus.
// Findings in the "changed" scope are penalized most heavily.
func ComputeHealthScore(findings []DedupedFinding) HealthScore {
	score := 100.0

	for i := range findings {
		f := &findings[i]

		// basePenalty is calibrated so that the new formula produces similar
		// deductions to the old per-risk formula:
		//   critical changed: 3.0 × 3.0 × 1.0 = 9  (old: 20, but gravity makes composite ~5+)
		//   warning changed:  3.0 × 2.0 × 1.0 = 6  (old: 8)
		//   info changed:     3.0 × 1.0 × 1.0 = 3  (old: 2, slightly higher)
		var basePenalty float64
		switch f.Scope {
		case ScopeChanged:
			basePenalty = 3.0
		case ScopeExisting:
			basePenalty = 1.0
		default: // codebase
			basePenalty = 0.25
		}

		score -= basePenalty * f.CompositeSeverity() * f.Consensus()
	}

	if score < 0 {
		score = 0
	}

	intScore := int(score + 0.5) // round
	return HealthScore{
		Score:       intScore,
		Grade:       scoreToGrade(intScore),
		Verdict:     scoreToVerdict(intScore),
		Description: scoreToDescription(intScore),
	}
}

// scoreToGrade uses the standard US university grading scale.
func scoreToGrade(score int) string {
	switch {
	case score >= 93:
		return "A"
	case score >= 90:
		return "A-"
	case score >= 87:
		return "B+"
	case score >= 83:
		return "B"
	case score >= 80:
		return "B-"
	case score >= 77:
		return "C+"
	case score >= 73:
		return "C"
	case score >= 70:
		return "C-"
	case score >= 67:
		return "D+"
	case score >= 63:
		return "D"
	case score >= 60:
		return "D-"
	default:
		return "F"
	}
}

func scoreToVerdict(score int) string {
	switch {
	case score >= 90:
		return VerdictApprove
	case score >= 80:
		return VerdictSuggestions
	case score >= 60:
		return VerdictRequestChanges
	default:
		return VerdictDiscuss
	}
}

func scoreToDescription(score int) string {
	switch {
	case score >= 93:
		return fmt.Sprintf("%d/100 — Excellent, no significant issues", score)
	case score >= 90:
		return fmt.Sprintf("%d/100 — Very good, minor suggestions only", score)
	case score >= 83:
		return fmt.Sprintf("%d/100 — Good, a few things to address", score)
	case score >= 77:
		return fmt.Sprintf("%d/100 — Acceptable, some issues to consider", score)
	case score >= 70:
		return fmt.Sprintf("%d/100 — Below average, several issues to fix", score)
	case score >= 60:
		return fmt.Sprintf("%d/100 — Needs work, multiple concerns", score)
	default:
		return fmt.Sprintf("%d/100 — Significant issues, needs rethinking", score)
	}
}

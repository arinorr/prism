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

		var basePenalty float64
		switch f.Scope {
		case ScopeChanged:
			basePenalty = 8.0
		case ScopeExisting:
			basePenalty = 2.0
		default: // codebase
			basePenalty = 0.5
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

func scoreToGrade(score int) string {
	switch {
	case score >= 95:
		return "A+"
	case score >= 90:
		return "A"
	case score >= 80:
		return "B+"
	case score >= 70:
		return "B"
	case score >= 60:
		return "C"
	case score >= 40:
		return "D"
	default:
		return "F"
	}
}

func scoreToVerdict(score int) string {
	switch {
	case score >= 90:
		return VerdictApprove
	case score >= 70:
		return VerdictSuggestions
	case score >= 40:
		return VerdictRequestChanges
	default:
		return VerdictDiscuss
	}
}

func scoreToDescription(score int) string {
	switch {
	case score >= 95:
		return fmt.Sprintf("%d/100 — Excellent, no significant issues", score)
	case score >= 90:
		return fmt.Sprintf("%d/100 — Very good, minor suggestions only", score)
	case score >= 80:
		return fmt.Sprintf("%d/100 — Good, a few things to address", score)
	case score >= 70:
		return fmt.Sprintf("%d/100 — Acceptable, several issues to fix", score)
	case score >= 60:
		return fmt.Sprintf("%d/100 — Needs work, multiple concerns", score)
	case score >= 40:
		return fmt.Sprintf("%d/100 — Significant issues found", score)
	default:
		return fmt.Sprintf("%d/100 — Major problems, needs rethinking", score)
	}
}

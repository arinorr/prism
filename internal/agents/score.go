// Package agents — score.go computes a PR health score from deduped findings.
package agents

import "fmt"

// HealthScore summarizes the overall quality of a PR based on findings.
type HealthScore struct {
	Score       int    `json:"score"`       // 0-100
	Grade       string `json:"grade"`       // A+, A, B+, B, C, D, F
	Verdict     string `json:"verdict"`     // approve, approve with suggestions, request changes, needs discussion
	Description string `json:"description"` // human-readable one-liner
}

// ComputeHealthScore computes a 0-100 health score from deduped findings.
// Findings in the "changed" scope (this PR) are penalized most heavily.
// Vote count amplifies the deduction (high-confidence findings hurt more).
func ComputeHealthScore(findings []DedupedFinding) HealthScore {
	score := 100.0

	for i := range findings {
		f := &findings[i]
		// Weight by vote confidence: a 7/7 finding deducts fully, a 1/7 deducts ~14%.
		weight := 1.0
		if f.TotalAgents > 0 {
			weight = float64(f.VoteCount) / float64(f.TotalAgents)
		}

		var deduction float64
		switch f.Scope {
		case ScopeChanged:
			switch f.Risk {
			case RiskCritical:
				deduction = 20
			case RiskWarning:
				deduction = 8
			default:
				deduction = 2
			}
		case ScopeExisting:
			switch f.Risk {
			case RiskCritical:
				deduction = 5
			case RiskWarning:
				deduction = 2
			default:
				deduction = 0.5
			}
		default: // codebase
			deduction = 1
		}

		score -= deduction * weight
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
		return "approve"
	case score >= 70:
		return "approve with suggestions"
	case score >= 40:
		return "request changes"
	default:
		return "needs discussion"
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

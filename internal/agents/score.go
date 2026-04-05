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
// Uses two independent signals:
//   - Confidence (0.0-1.0): how sure we are the issue is real (gates the deduction)
//   - CompositeSeverity (1.0-3.0): weighted consensus on how bad the issue is (scales the deduction)
//
// Findings in the "changed" scope are penalized most heavily.
func ComputeHealthScore(findings []DedupedFinding) HealthScore {
	score := 100.0

	for i := range findings {
		f := &findings[i]

		// Use Confidence if set, otherwise derive from vote ratio for backward compat.
		confidence := f.Confidence
		if confidence == 0 && f.TotalAgents > 0 {
			confidence = float64(f.VoteCount) / float64(f.TotalAgents)
		}

		// Use CompositeSeverity if set, otherwise derive from Risk label.
		severity := f.CompositeSeverity
		if severity == 0 {
			severity = SeverityNumeric(f.Risk)
		}

		var basePenalty float64
		switch f.Scope {
		case ScopeChanged:
			basePenalty = 8.0
		case ScopeExisting:
			basePenalty = 2.0
		default: // codebase
			basePenalty = 0.5
		}

		score -= basePenalty * severity * confidence
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

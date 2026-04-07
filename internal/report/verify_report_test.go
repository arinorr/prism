package report

import (
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
)

func verifyTestData() *Data {
	return &Data{
		PR: &gh.PR{
			Number: "42",
			Title:  "Test PR with verification",
			Files:  []gh.FileChange{{Path: "handler.go"}},
		},
		Result: &agents.ReviewResult{
			Summary: "Review complete.",
			DedupedFindings: []agents.DedupedFinding{
				{
					Finding: agents.Finding{
						File:     "handler.go",
						Line:     10,
						Risk:     agents.RiskWarning,
						Category: "bug",
						Scope:    agents.ScopeChanged,
						Summary:  "confirmed issue",
						Detail:   "This is a real issue.",
					},
					VoteCount:          3,
					TotalAgents:        7,
					Voters:             []string{"solver", "sentinel", "architect"},
					VerificationStatus: agents.StatusConfirmed,
					VerificationReason: "verified by code context",
				},
				{
					Finding: agents.Finding{
						File:     "handler.go",
						Line:     20,
						Risk:     agents.RiskInfo,
						Category: "style",
						Scope:    agents.ScopeChanged,
						Summary:  "downgraded issue",
						Detail:   "Originally warning, downgraded to info.",
					},
					VoteCount:          2,
					TotalAgents:        7,
					Voters:             []string{"editor", "know-it-all"},
					VerificationStatus: agents.StatusDowngraded,
					VerificationReason: "less severe than reported",
				},
			},
			HealthScore: agents.HealthScore{
				Score:       85,
				Grade:       "B+",
				Verdict:     "approve with suggestions",
				Description: "85/100",
			},
		},
		Roles:           []string{"Solver", "Sentinel", "Editor"},
		Duration:        "30s",
		Usage:           llm.Usage{InputTokens: 50000, OutputTokens: 5000, CostUSD: 0.25},
		DismissedCount:  3,
		DowngradedCount: 1,
	}
}

func TestMarkdown_VerificationSummary(t *testing.T) {
	d := verifyTestData()
	output := Markdown(d)

	if !strings.Contains(output, "dismissed 3 false positive(s)") {
		t.Error("expected dismissed count in markdown")
	}
	if !strings.Contains(output, "downgraded 1 finding(s)") {
		t.Error("expected downgraded count in markdown")
	}
}

func TestMarkdown_VerificationBadges(t *testing.T) {
	d := verifyTestData()
	output := Markdown(d)

	if !strings.Contains(output, "✅") {
		t.Error("expected confirmed badge in markdown")
	}
	if !strings.Contains(output, "⬇️") {
		t.Error("expected downgraded badge in markdown")
	}
}

func TestMarkdown_VerifierError(t *testing.T) {
	d := verifyTestData()
	d.VerifierError = "context deadline exceeded"
	output := Markdown(d)

	if !strings.Contains(output, "Verification error") {
		t.Error("expected verifier error in markdown")
	}
	if !strings.Contains(output, "context deadline exceeded") {
		t.Error("expected error message in markdown")
	}
}

func TestMarkdown_NoVerification(t *testing.T) {
	d := verifyTestData()
	d.DismissedCount = 0
	d.DowngradedCount = 0

	// Clear verification status from findings.
	for i := range d.Result.DedupedFindings {
		d.Result.DedupedFindings[i].VerificationStatus = ""
	}

	output := Markdown(d)

	if strings.Contains(output, "Verifier") {
		t.Error("should not mention verifier when no verification happened")
	}
}

func TestHTML_VerificationFields(t *testing.T) {
	d := verifyTestData()
	output, err := HTML(d)
	if err != nil {
		t.Fatal(err)
	}

	if len(output) == 0 {
		t.Error("expected non-empty HTML output")
	}
	// HTML should render without error even with verification data.
}

func TestJSON_VerificationStatus(t *testing.T) {
	d := verifyTestData()
	output, err := JSON(d)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, `"verification_status"`) {
		t.Error("expected verification_status in JSON output")
	}
	if !strings.Contains(output, `"confirmed"`) {
		t.Error("expected confirmed status in JSON")
	}
	if !strings.Contains(output, `"downgraded"`) {
		t.Error("expected downgraded status in JSON")
	}
	if !strings.Contains(output, `"dismissed_count"`) {
		t.Error("expected dismissed_count in JSON")
	}
}

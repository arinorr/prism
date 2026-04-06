package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
)

// richTestData returns a Data with deduped findings, failed agents, health
// score, suggestions, and usage — exercises all report code paths.
func richTestData() *Data {
	return &Data{
		PR: &gh.PR{
			Number: "99",
			Title:  "Refactor auth middleware",
			Files: []gh.FileChange{
				{Path: "auth/middleware.go", Status: "modified"},
				{Path: "auth/session.go", Status: "modified"},
				{Path: "auth/login_test.go", Status: "added"},
			},
		},
		Result: &agents.ReviewResult{
			Summary: "3 agents found 5 issues including 1 critical.",
			HealthScore: agents.HealthScore{
				Score:       65,
				Grade:       "C",
				Verdict:     "request changes",
				Description: "65/100 — Needs improvement",
			},
			Findings: []agents.Finding{
				{File: "auth/middleware.go", Line: 42, Risk: "critical", Category: "security", Scope: "changed", Summary: "Missing CSRF token check", Detail: "The middleware does not verify CSRF tokens.", Role: "sentinel", Confidence: 0.95},
				{File: "auth/middleware.go", Line: 42, Risk: "critical", Category: "security", Scope: "changed", Summary: "CSRF protection absent", Detail: "No CSRF validation in auth flow.", Role: "know-it-all", Confidence: 0.9},
				{File: "auth/session.go", Line: 15, Risk: "warning", Category: "design", Scope: "changed", Summary: "Session timeout too long", Detail: "24h session timeout is excessive.", Role: "architect", Confidence: 0.8},
				{File: "auth/login_test.go", Line: 0, Risk: "info", Category: "testing", Scope: "changed", Summary: "Missing edge case tests", Detail: "No tests for expired sessions.", Role: "test-engineer", Confidence: 0.7},
				{File: "", Line: 0, Risk: "info", Category: "design", Scope: "codebase", Summary: "Consider rate limiting", Detail: "No rate limiting on auth endpoints.", Role: "solver", Confidence: 0.6},
			},
			DedupedFindings: []agents.DedupedFinding{
				{
					Finding:     agents.Finding{File: "auth/middleware.go", Line: 42, Risk: "critical", Category: "security", Scope: "changed", Summary: "Missing CSRF token check", Detail: "The middleware does not verify CSRF tokens.", CodeExample: "// Add: csrf.Protect(r)"},
					VoteCount:   2,
					TotalAgents: 5,
					Voters:      []string{"sentinel", "know-it-all"},
				},
				{
					Finding:     agents.Finding{File: "auth/session.go", Line: 15, Risk: "warning", Category: "design", Scope: "changed", Summary: "Session timeout too long", Detail: "24h session timeout is excessive."},
					VoteCount:   1,
					TotalAgents: 5,
					Voters:      []string{"architect"},
				},
				{
					Finding:     agents.Finding{File: "auth/login_test.go", Line: 0, Risk: "info", Category: "testing", Scope: "changed", Summary: "Missing edge case tests", Detail: "No tests for expired sessions."},
					VoteCount:   1,
					TotalAgents: 5,
					Voters:      []string{"test-engineer"},
				},
				{
					Finding:     agents.Finding{File: "", Line: 0, Risk: "info", Category: "design", Scope: "codebase", Summary: "Consider rate limiting", Detail: "No rate limiting on auth endpoints."},
					VoteCount:   1,
					TotalAgents: 5,
					Voters:      []string{"solver"},
				},
			},
			Suggestions: []gh.Suggestion{
				{File: "auth/middleware.go", Line: 42, Body: "Add CSRF check", Role: "sentinel"},
				{File: "auth/session.go", Line: 15, Body: "Reduce timeout", Role: "architect"},
			},
			FailedAgents: []string{"Optimizer"},
		},
		Roles:    []string{"Sentinel", "Know-It-All", "Architect", "Solver", "Test Engineer"},
		Duration: "32s",
		Usage:    llm.Usage{InputTokens: 45000, OutputTokens: 8000, CostUSD: 2.15},
	}
}

// TestConsistency_AllFormatsContainSameFindings verifies that markdown, HTML,
// and JSON reports all contain the same key data when given identical input.
func TestConsistency_AllFormatsContainSameFindings(t *testing.T) {
	t.Parallel()
	d := richTestData()

	md := Markdown(d)
	html, htmlErr := HTML(d)
	if htmlErr != nil {
		t.Fatalf("HTML generation failed: %v", htmlErr)
	}
	jsonStr, jsonErr := JSON(d)
	if jsonErr != nil {
		t.Fatalf("JSON generation failed: %v", jsonErr)
	}

	// All formats should contain the PR number.
	for name, output := range map[string]string{"markdown": md, "html": html, "json": jsonStr} {
		if !strings.Contains(output, "99") {
			t.Errorf("%s: missing PR number 99", name)
		}
	}

	// All formats should contain the critical finding summary.
	for name, output := range map[string]string{"markdown": md, "html": html, "json": jsonStr} {
		if !strings.Contains(output, "CSRF") {
			t.Errorf("%s: missing CSRF finding", name)
		}
	}

	// All formats should reference the failed agent.
	for name, output := range map[string]string{"markdown": md, "html": html, "json": jsonStr} {
		if !strings.Contains(output, "Optimizer") {
			t.Errorf("%s: missing failed agent 'Optimizer'", name)
		}
	}

	// JSON should have consistent counts.
	var parsed struct {
		HealthScore struct {
			Score int    `json:"score"`
			Grade string `json:"grade"`
		} `json:"health_score"`
		DedupedFindings []struct {
			VoteCount int `json:"vote_count"`
		} `json:"deduped_findings"`
		FailedAgents []string `json:"failed_agents"`
		Usage        struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}
	if parsed.HealthScore.Score != 65 {
		t.Errorf("JSON health score %d != expected 65", parsed.HealthScore.Score)
	}
	if parsed.HealthScore.Grade != "C" {
		t.Errorf("JSON grade %q != expected 'C'", parsed.HealthScore.Grade)
	}
	if len(parsed.DedupedFindings) != 4 {
		t.Errorf("JSON deduped findings count %d != expected 4", len(parsed.DedupedFindings))
	}
	if len(parsed.FailedAgents) != 1 || parsed.FailedAgents[0] != "Optimizer" {
		t.Errorf("JSON failed agents %v != expected [Optimizer]", parsed.FailedAgents)
	}
	if parsed.Usage.InputTokens != 45000 {
		t.Errorf("JSON usage input tokens %d != expected 45000", parsed.Usage.InputTokens)
	}

	// Markdown and HTML should reflect the health score.
	if !strings.Contains(md, "C") || !strings.Contains(md, "request changes") {
		t.Error("markdown missing health score grade/verdict")
	}
	if !strings.Contains(html, "65") {
		t.Error("HTML missing health score value")
	}
}

// TestConsistency_EmptyFindingsAllFormats verifies all three formats handle
// a review with zero findings gracefully.
func TestConsistency_EmptyFindingsAllFormats(t *testing.T) {
	t.Parallel()
	d := &Data{
		PR:     &gh.PR{Number: "1", Title: "Clean PR", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{Summary: "No issues found.", HealthScore: agents.HealthScore{Score: 100, Grade: "A+", Verdict: "approve", Description: "Perfect"}},
		Roles:  []string{"Sentinel"},
	}

	md := Markdown(d)
	if !strings.Contains(md, "No issues found") {
		t.Error("markdown should contain summary for clean PR")
	}

	html, err := HTML(d)
	if err != nil {
		t.Fatalf("HTML failed: %v", err)
	}
	if !strings.Contains(html, "100") {
		t.Error("HTML should contain score for clean PR")
	}

	jsonStr, err := JSON(d)
	if err != nil {
		t.Fatalf("JSON failed: %v", err)
	}
	if !strings.Contains(jsonStr, `"score": 100`) {
		t.Error("JSON should contain score 100 for clean PR")
	}
}

// TestConsistency_SpecialCharsInFindings verifies that HTML-sensitive
// characters in findings don't break report generation.
func TestConsistency_SpecialCharsInFindings(t *testing.T) {
	t.Parallel()
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Fix <script>alert('xss')</script>", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: `Check for "quotes" & <angles> in output.`,
			Findings: []agents.Finding{
				{File: "a.go", Line: 1, Risk: "warning", Category: "security", Scope: "changed",
					Summary: `Found <img onerror="alert(1)"> in template`,
					Detail:  "The template uses `html/template` which escapes this, but verify.",
					Role:    "sentinel", Confidence: 0.8},
			},
			HealthScore: agents.HealthScore{Score: 80, Grade: "B+", Verdict: "approve with suggestions", Description: "Good"},
		},
		Roles: []string{"Sentinel"},
	}

	// HTML should not contain raw script tags — bluemonday sanitizes.
	html, err := HTML(d)
	if err != nil {
		t.Fatalf("HTML generation failed: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Error("HTML output contains unsanitized <script> tag")
	}

	// Markdown should pass through special chars (it's text, not HTML).
	md := Markdown(d)
	if !strings.Contains(md, "<img") {
		t.Error("markdown should preserve angle brackets in findings")
	}

	// JSON should escape properly.
	jsonStr, err := JSON(d)
	if err != nil {
		t.Fatalf("JSON generation failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("JSON with special chars should be valid: %v", err)
	}
}

// TestConsistency_GeneralFileFinding verifies that findings with empty file
// names are rendered as "(general)" in reports.
func TestConsistency_GeneralFileFinding(t *testing.T) {
	t.Parallel()
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Summary.",
			Findings: []agents.Finding{
				{File: "", Line: 0, Risk: "info", Category: "design", Scope: "codebase",
					Summary: "Consider adding a README", Detail: "d", Role: "editor", Confidence: 0.7},
			},
			HealthScore: agents.HealthScore{Score: 95, Grade: "A", Verdict: "approve", Description: "Good"},
		},
		Roles: []string{"Editor"},
	}

	md := Markdown(d)
	if !strings.Contains(md, "(general)") {
		t.Error("markdown should show '(general)' for findings with empty file")
	}
}

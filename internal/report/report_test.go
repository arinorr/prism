package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
)

func testData() *Data {
	return &Data{
		PR: &gh.PR{
			Number: "42",
			Title:  "Fix the widget",
			Files: []gh.FileChange{
				{Path: "widget.go", Status: "modified"},
				{Path: "widget_test.go", Status: "modified"},
			},
		},
		Result: &agents.ReviewResult{
			Summary: "Overall looks good with minor issues.",
			Findings: []agents.Finding{
				{File: "widget.go", Line: 10, Risk: "critical", Category: "bug", Scope: "changed", Summary: "Nil pointer", Detail: "Check for nil before dereferencing.", Role: "solver"},
				{File: "widget.go", Line: 25, Risk: "warning", Category: "design", Scope: "changed", Summary: "Long function", Detail: "Consider extracting a helper.", Role: "editor"},
				{File: "widget_test.go", Line: 5, Risk: "info", Category: "testing", Scope: "existing", Summary: "Missing edge case", Detail: "Add a test for empty input.", Role: "test-engineer"},
			},
			HealthScore: agents.HealthScore{Score: 72, Grade: "B", Verdict: "approve with suggestions", Description: "72/100 — Acceptable, several issues to fix"},
			Suggestions: []gh.Suggestion{
				{File: "widget.go", Line: 10, Body: "Check for nil", Role: "solver"},
			},
		},
		Roles:    []string{"Solver", "Editor", "Test Engineer"},
		Duration: "45s",
	}
}

func TestMarkdown_ContainsHeader(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "# Prism Review: PR #42") {
		t.Error("markdown should contain PR header")
	}
	if !strings.Contains(md, "Fix the widget") {
		t.Error("markdown should contain PR title")
	}
}

func TestMarkdown_ContainsSummary(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "Overall looks good") {
		t.Error("markdown should contain synthesis summary")
	}
}

func TestMarkdown_ContainsFindings(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "Nil pointer") {
		t.Error("markdown should contain critical finding")
	}
	if !strings.Contains(md, "Long function") {
		t.Error("markdown should contain warning finding")
	}
	if !strings.Contains(md, "`widget.go`") {
		t.Error("markdown should group by file")
	}
	// Should have Category column.
	if !strings.Contains(md, "Category") {
		t.Error("markdown should contain Category column header")
	}
}

func TestMarkdown_ContainsMetadata(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "Solver, Editor, Test Engineer") {
		t.Error("markdown should list roles")
	}
	if !strings.Contains(md, "45s") {
		t.Error("markdown should contain duration")
	}
}

func TestMarkdown_ContainsSuggestionCount(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "1 inline suggestions") {
		t.Error("markdown should mention suggestion count")
	}
}

func TestHTML_ValidOutput(t *testing.T) {
	html, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should contain doctype")
	}
	if !strings.Contains(html, "PR #42") {
		t.Error("HTML should contain PR number")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("HTML should be properly closed")
	}
}

func TestJSON_ValidOutput(t *testing.T) {
	j, err := JSON(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(j, `"number": "42"`) {
		t.Error("JSON should contain PR number")
	}
	if !strings.Contains(j, `"findings"`) {
		t.Error("JSON should contain findings array")
	}
	if !strings.Contains(j, `"suggestions"`) {
		t.Error("JSON should contain suggestions array")
	}
}

func TestGroupByFile_SortsBySeverity(t *testing.T) {
	findings := []agents.Finding{
		{File: "a.go", Risk: "info", Summary: "info item"},
		{File: "a.go", Risk: "critical", Summary: "critical item"},
		{File: "a.go", Risk: "warning", Summary: "warning item"},
	}
	groups := groupByFile(findings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].findings[0].Risk != "critical" {
		t.Errorf("expected critical first, got %q", groups[0].findings[0].Risk)
	}
	if groups[0].findings[1].Risk != "warning" {
		t.Errorf("expected warning second, got %q", groups[0].findings[1].Risk)
	}
}

func TestSeverityBadge(t *testing.T) {
	if !strings.Contains(severityBadge("critical"), "critical") {
		t.Error("critical badge should contain 'critical'")
	}
	if !strings.Contains(severityBadge("warning"), "warning") {
		t.Error("warning badge should contain 'warning'")
	}
	if !strings.Contains(severityBadge("info"), "info") {
		t.Error("info badge should contain 'info'")
	}
}

func TestAgentColorClass(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"know-it-all", "agent-purple"},
		{"architect", "agent-indigo"},
		{"solver", "agent-teal"},
		{"editor", "agent-orange"},
		{"optimizer", "agent-green"},
		{"sentinel", "agent-red"},
		{"test-engineer", "agent-blue"},
		{"unknown-role", "agent-default"},
	}
	for _, tt := range tests {
		if got := agentColorClass(tt.role); got != tt.want {
			t.Errorf("agentColorClass(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestHTML_ContainsAgentBadges(t *testing.T) {
	// Use deduped data which has agent details with badges.
	out, err := HTML(dedupedTestData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "agent-badge") {
		t.Error("HTML should contain agent-badge class")
	}
	if !strings.Contains(out, "architect") {
		t.Error("HTML should contain architect agent name")
	}
}

func TestHTML_UsesTemplate(t *testing.T) {
	// Verify the template renders without errors for various data shapes.
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test"},
		Result: &agents.ReviewResult{
			Summary:  "Clean.",
			Findings: nil,
		},
		Roles: []string{},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error for empty findings: %v", err)
	}
	if !strings.Contains(out, "PR #1") {
		t.Error("HTML should contain PR number")
	}
}

func TestHTML_DashboardRendered(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Dashboard should show verdict and risk badges.
	if !strings.Contains(out, "dash-verdict") {
		t.Error("HTML should contain verdict in dashboard")
	}
	if !strings.Contains(out, "dash-risks") {
		t.Error("HTML should contain risk badges in dashboard")
	}
}

func TestMarkdown_NoLine0InDetails(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test summary.",
			Findings: []agents.Finding{
				{File: "a.go", Line: 0, Risk: "warning", Summary: "General issue", Detail: "This is a general warning.", Role: "editor"},
				{File: "a.go", Line: 10, Risk: "critical", Summary: "Specific issue", Detail: "This is on line 10.", Role: "solver"},
			},
		},
		Roles: []string{"Editor", "Solver"},
	}
	md := Markdown(d)
	// The detail for line 0 should NOT contain "line 0".
	if strings.Contains(md, "line 0") {
		t.Error("markdown detail should not print 'line 0' for findings without a line number")
	}
	// The detail for line 10 should contain "line 10".
	if !strings.Contains(md, "line 10") {
		t.Error("markdown detail should print 'line 10' for findings with a line number")
	}
}

func TestJSON_NilSlicesSerializeAsEmptyArrays(t *testing.T) {
	d := &Data{
		PR:     &gh.PR{Number: "1", Title: "Test"},
		Result: &agents.ReviewResult{Summary: "ok"},
		// Roles, Findings, Suggestions are all nil.
	}
	j, err := JSON(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(j), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	for _, key := range []string{"findings", "suggestions", "roles"} {
		raw, ok := parsed[key]
		if !ok {
			t.Errorf("JSON missing key %q", key)
			continue
		}
		if string(raw) == "null" {
			t.Errorf("JSON key %q should be [] not null", key)
		}
	}
}

func TestHTML_ContainsDashboard(t *testing.T) {
	html, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "dashboard") {
		t.Error("HTML should contain dashboard section")
	}
	if !strings.Contains(html, "dash-card") {
		t.Error("HTML should contain dashboard cards")
	}
	if !strings.Contains(html, "badge-warning") {
		t.Error("HTML should contain warning badge in dashboard")
	}
}

func TestHTML_ContainsFileGroups(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "file-group") {
		t.Error("HTML should contain file group elements")
	}
	if !strings.Contains(out, "widget.go") {
		t.Error("HTML should contain file names")
	}
}

func TestHTML_EscapesXSSInFindings(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "<script>alert('xss')</script>", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test summary.",
			Findings: []agents.Finding{
				{File: "<img src=x>.go", Line: 10, Risk: "critical", Summary: "<b>bold xss</b>", Detail: "<script>alert(1)</script>", Role: "sentinel"},
			},
		},
		Roles: []string{"Sentinel"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify that raw HTML is escaped, not rendered.
	if strings.Contains(out, "<script>alert") {
		t.Error("HTML output should escape <script> tags in findings")
	}
	if strings.Contains(out, "<img src=x>") {
		t.Error("HTML output should escape <img> tags in file names")
	}
	if strings.Contains(out, "<b>bold xss</b>") {
		t.Error("HTML output should escape <b> tags in summaries")
	}
	// Verify escaped versions are present.
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("HTML output should contain escaped script tags")
	}
}

func TestGroupByFile_EmptyFile(t *testing.T) {
	findings := []agents.Finding{
		{File: "", Risk: "info", Summary: "general note"},
	}
	groups := groupByFile(findings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].file != "(general)" {
		t.Errorf("expected file name '(general)', got %q", groups[0].file)
	}
}

func TestGroupByFile_MultipleFiles(t *testing.T) {
	findings := []agents.Finding{
		{File: "b.go", Risk: "info", Summary: "b note"},
		{File: "a.go", Risk: "warning", Summary: "a note"},
	}
	groups := groupByFile(findings)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	// Groups should be sorted alphabetically by file.
	if groups[0].file != "a.go" {
		t.Errorf("expected first group 'a.go', got %q", groups[0].file)
	}
}

func dedupedTestData() *Data {
	return &Data{
		PR: &gh.PR{
			Number: "99",
			Title:  "Deduped test",
			Files:  []gh.FileChange{{Path: "a.go"}},
		},
		Result: &agents.ReviewResult{
			Summary: "Summary with deduped findings.",
			DedupedFindings: []agents.DedupedFinding{
				{
					Finding:     agents.Finding{File: "a.go", Line: 10, Risk: "critical", Category: "bug", Scope: "changed", Summary: "missing timeout", Detail: "Add context.WithTimeout", CodeExample: "// before\nctx := context.Background()\n// after\nctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)"},
					VoteCount:   5,
					TotalAgents: 7,
					Voters:      []string{"architect", "solver", "sentinel", "optimizer", "editor"},
					AgentDetails: []agents.AgentDetail{
						{Role: "architect", Detail: "Architect says add timeout", CodeExample: "ctx, cancel := context.WithTimeout(ctx, 5*time.Second)"},
						{Role: "solver", Detail: "Solver agrees, timeout needed"},
						{Role: "sentinel", Detail: "Sentinel: security concern without timeout"},
					},
				},
				{
					Finding:     agents.Finding{File: "a.go", Line: 50, Risk: "info", Category: "style", Scope: "existing", Summary: "style note", Detail: "Minor style"},
					VoteCount:   1,
					TotalAgents: 7,
					Voters:      []string{"editor"},
					AgentDetails: []agents.AgentDetail{
						{Role: "editor", Detail: "Minor style issue"},
					},
				},
			},
			HealthScore:  agents.HealthScore{Score: 65, Grade: "C", Verdict: "request changes", Description: "65/100 — Needs work, multiple concerns"},
			FailedAgents: []string{"Test Engineer", "Know-It-All"},
		},
		Roles:    []string{"Architect", "Solver"},
		Duration: "2m30s",
	}
}

func TestMarkdown_DedupedFindings(t *testing.T) {
	d := dedupedTestData()
	output := Markdown(d)
	if !strings.Contains(output, "5/7") {
		t.Error("expected vote count '5/7' in deduped markdown")
	}
	if !strings.Contains(output, "Votes") {
		t.Error("expected 'Votes' column header in deduped markdown")
	}
	if !strings.Contains(output, "architect, solver, sentinel, optimizer, editor") {
		t.Error("expected voter names in detail")
	}
}

func TestMarkdown_FailedAgents(t *testing.T) {
	d := dedupedTestData()
	output := Markdown(d)
	if !strings.Contains(output, "2 agent(s) failed") {
		t.Error("expected failed agents notice in markdown")
	}
	if !strings.Contains(output, "Test Engineer") {
		t.Error("expected failed agent name in markdown")
	}
}

func TestHTML_FailedAgents(t *testing.T) {
	d := dedupedTestData()
	output, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "2 agent(s) failed") {
		t.Error("expected failed agents notice in HTML")
	}
}

func TestHTML_DedupedFindings(t *testing.T) {
	d := dedupedTestData()
	output, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "vote-count") {
		t.Error("HTML should contain vote-count class for deduped findings")
	}
	if !strings.Contains(output, "5/7") {
		t.Error("HTML should contain vote count '5/7' for deduped findings")
	}
}

func TestHTML_DedupedFindingsPreferredOverRaw(t *testing.T) {
	// When both Findings and DedupedFindings are present, deduped should win.
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test.",
			Findings: []agents.Finding{
				{File: "a.go", Line: 10, Risk: "warning", Category: "design", Scope: "changed", Summary: "raw finding", Role: "solver"},
			},
			DedupedFindings: []agents.DedupedFinding{
				{
					Finding:     agents.Finding{File: "a.go", Line: 10, Risk: "warning", Category: "design", Scope: "changed", Summary: "deduped finding"},
					VoteCount:   3,
					TotalAgents: 5,
					Voters:      []string{"solver", "architect", "sentinel"},
				},
			},
			HealthScore: agents.HealthScore{Score: 80, Grade: "B+", Verdict: "approve with suggestions", Description: "80/100 — Good"},
		},
		Roles: []string{"Test"},
	}
	output, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "deduped finding") {
		t.Error("HTML should show deduped finding when both are present")
	}
	if strings.Contains(output, "raw finding") {
		t.Error("HTML should not show raw finding when deduped findings are present")
	}
	if !strings.Contains(output, "3/5") {
		t.Error("HTML should show vote count 3/5")
	}
}

func TestJSON_DedupedFindings(t *testing.T) {
	d := dedupedTestData()
	output, err := JSON(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "deduped_findings") {
		t.Error("expected deduped_findings in JSON")
	}
	if !strings.Contains(output, "vote_count") {
		t.Error("expected vote_count in JSON")
	}
	if !strings.Contains(output, "failed_agents") {
		t.Error("expected failed_agents in JSON")
	}
	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(output), &parsed); jsonErr != nil {
		t.Fatalf("JSON output is not valid: %v", jsonErr)
	}
}

func TestGroupDedupedByFile(t *testing.T) {
	findings := []agents.DedupedFinding{
		{Finding: agents.Finding{File: "b.go", Line: 1, Risk: "info"}, VoteCount: 1},
		{Finding: agents.Finding{File: "a.go", Line: 10, Risk: "critical"}, VoteCount: 3},
		{Finding: agents.Finding{File: "a.go", Line: 20, Risk: "warning"}, VoteCount: 2},
	}
	groups := groupDedupedByFile(findings)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].file != "a.go" {
		t.Errorf("expected first group 'a.go', got %q", groups[0].file)
	}
	if len(groups[0].findings) != 2 {
		t.Errorf("expected 2 findings in a.go group, got %d", len(groups[0].findings))
	}
	// Within a.go, sorted by vote count desc.
	if groups[0].findings[0].VoteCount != 3 {
		t.Errorf("expected first finding to have 3 votes, got %d", groups[0].findings[0].VoteCount)
	}
}

func TestGroupDedupedByFile_EmptyFile(t *testing.T) {
	findings := []agents.DedupedFinding{
		{Finding: agents.Finding{File: "", Risk: "info"}, VoteCount: 1},
	}
	groups := groupDedupedByFile(findings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].file != "(general)" {
		t.Errorf("expected '(general)' for empty file, got %q", groups[0].file)
	}
}

func TestGroupDedupedByFile_MultipleFiles(t *testing.T) {
	findings := []agents.DedupedFinding{
		{Finding: agents.Finding{File: "c.go", Line: 1, Risk: "info"}, VoteCount: 1},
		{Finding: agents.Finding{File: "a.go", Line: 10, Risk: "critical"}, VoteCount: 5},
		{Finding: agents.Finding{File: "a.go", Line: 20, Risk: "warning"}, VoteCount: 2},
		{Finding: agents.Finding{File: "b.go", Line: 5, Risk: "critical"}, VoteCount: 3},
	}
	groups := groupDedupedByFile(findings)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// Sorted alphabetically by file.
	if groups[0].file != "a.go" || groups[1].file != "b.go" || groups[2].file != "c.go" {
		t.Errorf("expected [a.go, b.go, c.go], got [%s, %s, %s]", groups[0].file, groups[1].file, groups[2].file)
	}
}

func TestHTML_FailedAgentsBanner(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary:      "Partial.",
			FailedAgents: []string{"Sentinel", "Optimizer"},
		},
		Roles: []string{"Test"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "failed") {
		t.Error("HTML should contain failed agents banner")
	}
	if !strings.Contains(out, "Sentinel") {
		t.Error("HTML should list failed agent names")
	}
}

func TestMarkdown_NoFailedAgents(t *testing.T) {
	d := &Data{
		PR:     &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{Summary: "Clean."},
		Roles:  []string{"Test"},
	}
	out := Markdown(d)
	if strings.Contains(out, "failed") {
		t.Error("should not contain failed agents notice when none failed")
	}
}

func TestHTML_ContainsGauge(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<svg") {
		t.Error("HTML should contain SVG gauge element")
	}
	if !strings.Contains(out, "gauge-container") {
		t.Error("HTML should contain gauge-container class")
	}
	if !strings.Contains(out, "72") {
		t.Error("HTML gauge should display the score number")
	}
}

func TestHTML_ContainsDetailsElements(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<details") {
		t.Error("HTML should contain <details> elements for finding cards")
	}
	if !strings.Contains(out, "finding-card") {
		t.Error("HTML should contain finding-card class")
	}
}

func TestHTML_ContainsCategoryBadge(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "cat-bug") {
		t.Error("HTML should contain cat-bug category class")
	}
	if !strings.Contains(out, "cat-badge") {
		t.Error("HTML should contain cat-badge class")
	}
}

func TestHTML_ContainsCodeExample(t *testing.T) {
	d := dedupedTestData()
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<pre") {
		t.Error("HTML should contain <pre> for code examples")
	}
	if !strings.Contains(out, "code-example") {
		t.Error("HTML should contain code-example class")
	}
	if !strings.Contains(out, "context.WithTimeout") {
		t.Error("HTML should contain the code example content")
	}
}

func TestHTML_GroupsByScope(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Issues in this PR") {
		t.Error("HTML should contain 'Issues in this PR' scope section")
	}
	if !strings.Contains(out, "Pre-existing Issues") {
		t.Error("HTML should contain 'Pre-existing Issues' scope section")
	}
}

func TestHTML_SingleAgentNoAgentDetails(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test.",
			DedupedFindings: []agents.DedupedFinding{
				{
					Finding:     agents.Finding{File: "a.go", Line: 10, Risk: "warning", Category: "design", Scope: "changed", Summary: "single agent finding", Detail: "Only one agent saw this."},
					VoteCount:   1,
					TotalAgents: 3,
					Voters:      []string{"solver"},
					AgentDetails: []agents.AgentDetail{
						{Role: "solver", Detail: "Only one agent saw this."},
					},
				},
			},
			HealthScore: agents.HealthScore{Score: 85, Grade: "B+", Verdict: "approve with suggestions", Description: "85/100 — Good, a few things to address"},
		},
		Roles: []string{"Solver"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With a single agent, there should be no nested agent-detail <details> elements
	// (the CSS class definition in the stylesheet is fine, we check for the HTML element).
	if strings.Contains(out, `<details class="agent-detail"`) {
		t.Error("HTML with single agent should not contain nested agent-detail sections")
	}
}

func TestMarkdown_GroupsByScope(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "Issues in this PR") {
		t.Error("markdown should contain 'Issues in this PR' scope section")
	}
	if !strings.Contains(md, "Pre-existing Issues") {
		t.Error("markdown should contain 'Pre-existing Issues' scope section")
	}
}

func TestMarkdown_HealthScore(t *testing.T) {
	md := Markdown(testData())
	if !strings.Contains(md, "Health Score") {
		t.Error("markdown should contain health score header")
	}
	if !strings.Contains(md, "72/100") {
		t.Error("markdown should contain the score description")
	}
	if !strings.Contains(md, "(B)") {
		t.Error("markdown should contain the grade")
	}
}

func TestJSON_HealthScore(t *testing.T) {
	d := testData()
	output, err := JSON(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, `"health_score"`) {
		t.Error("JSON should contain health_score field")
	}
	if !strings.Contains(output, `"score": 72`) {
		t.Error("JSON should contain score value")
	}
	if !strings.Contains(output, `"grade": "B"`) {
		t.Error("JSON should contain grade value")
	}
}

func TestHTML_GaugeNeedleRotation(t *testing.T) {
	tests := []struct {
		name    string
		score   int
		wantMin int // minimum rotation (inclusive)
		wantMax int // maximum rotation (inclusive)
	}{
		{"score 10 points left", 10, -72, -72},
		{"score 50 points up", 50, 0, 0},
		{"score 100 points right", 100, 90, 90},
		{"score 46 points left of center", 46, -8, -7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := testData()
			d.Result.HealthScore.Score = tt.score
			out, err := HTML(d)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			rotation := int(float64(tt.score)*1.8) - 90
			expected := fmt.Sprintf("rotate(%d 100 110)", rotation)
			if !strings.Contains(out, expected) {
				t.Errorf("expected needle rotation %q in SVG, score=%d", expected, tt.score)
			}
		})
	}
}

func TestHTML_FilesOrderedBySeverity(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test.",
			DedupedFindings: []agents.DedupedFinding{
				{Finding: agents.Finding{File: "a.go", Line: 1, Risk: "info", Category: "style", Scope: "changed", Summary: "minor"}, VoteCount: 1, TotalAgents: 3, Voters: []string{"editor"}, AgentDetails: []agents.AgentDetail{{Role: "editor", Detail: "d"}}},
				{Finding: agents.Finding{File: "b.go", Line: 1, Risk: "critical", Category: "bug", Scope: "changed", Summary: "crash"}, VoteCount: 1, TotalAgents: 3, Voters: []string{"solver"}, AgentDetails: []agents.AgentDetail{{Role: "solver", Detail: "d"}}},
				{Finding: agents.Finding{File: "c.go", Line: 1, Risk: "warning", Category: "design", Scope: "changed", Summary: "design issue"}, VoteCount: 1, TotalAgents: 3, Voters: []string{"architect"}, AgentDetails: []agents.AgentDetail{{Role: "architect", Detail: "d"}}},
			},
			HealthScore: agents.HealthScore{Score: 60, Grade: "C", Verdict: "request changes"},
		},
		Roles: []string{"Solver", "Architect", "Editor"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// b.go (critical) should appear before c.go (warning) which should appear before a.go (info).
	bPos := strings.Index(out, "b.go")
	cPos := strings.Index(out, "c.go")
	aPos := strings.Index(out, "a.go")
	if bPos < 0 || cPos < 0 || aPos < 0 {
		t.Fatal("all three files should appear in the HTML output")
	}
	if bPos > cPos {
		t.Error("b.go (critical) should appear before c.go (warning)")
	}
	if cPos > aPos {
		t.Error("c.go (warning) should appear before a.go (info)")
	}
}

func TestHTML_DashboardConsolidated(t *testing.T) {
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should NOT have the old 3-card grid or Scope card.
	if strings.Contains(out, "dashboard-grid") {
		t.Error("should not have old dashboard-grid layout")
	}
	if strings.Contains(out, ">Scope<") {
		t.Error("should not have separate Scope card")
	}
	// Should have consolidated card with verdict and risk badges.
	if !strings.Contains(out, "dash-card-wide") {
		t.Error("should have wide consolidated dashboard card")
	}
	if !strings.Contains(out, "dash-risks") {
		t.Error("should have risk badges section")
	}
}

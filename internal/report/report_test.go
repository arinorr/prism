package report

import (
	"encoding/json"
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
				{File: "widget.go", Line: 10, Severity: "critical", Summary: "Nil pointer", Detail: "Check for nil before dereferencing.", Role: "solver"},
				{File: "widget.go", Line: 25, Severity: "warning", Summary: "Long function", Detail: "Consider extracting a helper.", Role: "editor"},
				{File: "widget_test.go", Line: 5, Severity: "info", Summary: "Missing edge case", Detail: "Add a test for empty input.", Role: "test-engineer"},
			},
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
		{File: "a.go", Severity: "info", Summary: "info item"},
		{File: "a.go", Severity: "critical", Summary: "critical item"},
		{File: "a.go", Severity: "warning", Summary: "warning item"},
	}
	groups := groupByFile(findings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].findings[0].Severity != "critical" {
		t.Errorf("expected critical first, got %q", groups[0].findings[0].Severity)
	}
	if groups[0].findings[1].Severity != "warning" {
		t.Errorf("expected warning second, got %q", groups[0].findings[1].Severity)
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
	out, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "agent-badge") {
		t.Error("HTML should contain agent-badge class")
	}
	if !strings.Contains(out, "solver") {
		t.Error("HTML should contain solver agent name")
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

func TestHTML_SanitizesSummaryXSS(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Good PR.\n\n<script>alert('xss')</script>\n\n[click](javascript:alert(1))",
		},
		Roles: []string{"Test"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "<script>") {
		t.Error("HTML summary should not contain <script> tags")
	}
	if strings.Contains(out, "javascript:") {
		t.Error("HTML summary should not contain javascript: URLs")
	}
	// Benign markdown should still render.
	if !strings.Contains(out, "Good PR.") {
		t.Error("HTML summary should still contain safe text")
	}
}

func TestHTML_SummaryPreservesSafeMarkdown(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "**Bold text** and `code` and [link](https://example.com)",
		},
		Roles: []string{"Test"},
	}
	out, err := HTML(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<strong>Bold text</strong>") {
		t.Error("HTML summary should preserve bold markdown")
	}
	if !strings.Contains(out, "<code>code</code>") {
		t.Error("HTML summary should preserve code markdown")
	}
}

func TestMarkdown_NoLine0InDetails(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test summary.",
			Findings: []agents.Finding{
				{File: "a.go", Line: 0, Severity: "warning", Summary: "General issue", Detail: "This is a general warning.", Role: "editor"},
				{File: "a.go", Line: 10, Severity: "critical", Summary: "Specific issue", Detail: "This is on line 10.", Role: "solver"},
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

func TestHTML_ContainsSeverityStats(t *testing.T) {
	html, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "stat-critical") {
		t.Error("HTML should contain critical stat")
	}
	if !strings.Contains(html, "stat-warning") {
		t.Error("HTML should contain warning stat")
	}
	if !strings.Contains(html, "stat-info") {
		t.Error("HTML should contain info stat")
	}
}

func TestHTML_ContainsFileGroups(t *testing.T) {
	html, err := HTML(testData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "file-group") {
		t.Error("HTML should contain file group elements")
	}
	if !strings.Contains(html, "widget.go") {
		t.Error("HTML should contain file names")
	}
}

func TestHTML_EscapesXSSInFindings(t *testing.T) {
	d := &Data{
		PR: &gh.PR{Number: "1", Title: "<script>alert('xss')</script>", Files: []gh.FileChange{{Path: "a.go"}}},
		Result: &agents.ReviewResult{
			Summary: "Test summary.",
			Findings: []agents.Finding{
				{File: "<img src=x>.go", Line: 10, Severity: "critical", Summary: "<b>bold xss</b>", Detail: "<script>alert(1)</script>", Role: "sentinel"},
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
		{File: "", Severity: "info", Summary: "general note"},
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
		{File: "b.go", Severity: "info", Summary: "b note"},
		{File: "a.go", Severity: "warning", Summary: "a note"},
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
					Finding:     agents.Finding{File: "a.go", Line: 10, Severity: "critical", Summary: "missing timeout", Detail: "Add context.WithTimeout"},
					VoteCount:   5,
					TotalAgents: 7,
					Voters:      []string{"architect", "solver", "sentinel", "optimizer", "editor"},
				},
				{
					Finding:     agents.Finding{File: "a.go", Line: 50, Severity: "info", Summary: "style note", Detail: "Minor style"},
					VoteCount:   1,
					TotalAgents: 7,
					Voters:      []string{"editor"},
				},
			},
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
		{Finding: agents.Finding{File: "b.go", Line: 1, Severity: "info"}, VoteCount: 1},
		{Finding: agents.Finding{File: "a.go", Line: 10, Severity: "critical"}, VoteCount: 3},
		{Finding: agents.Finding{File: "a.go", Line: 20, Severity: "warning"}, VoteCount: 2},
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
		{Finding: agents.Finding{File: "", Severity: "info"}, VoteCount: 1},
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
		{Finding: agents.Finding{File: "c.go", Line: 1, Severity: "info"}, VoteCount: 1},
		{Finding: agents.Finding{File: "a.go", Line: 10, Severity: "critical"}, VoteCount: 5},
		{Finding: agents.Finding{File: "a.go", Line: 20, Severity: "warning"}, VoteCount: 2},
		{Finding: agents.Finding{File: "b.go", Line: 5, Severity: "critical"}, VoteCount: 3},
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

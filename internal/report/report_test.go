package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/arinorr/shinobi/internal/agents"
	"github.com/arinorr/shinobi/internal/gh"
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
	if !strings.Contains(md, "# Shinobi Review: PR #42") {
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

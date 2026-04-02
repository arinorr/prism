package report

import (
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

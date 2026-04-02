package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
)

func TestParseReviewArgs_BasicPR(t *testing.T) {
	opts, err := parseReviewArgs([]string{"42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.prRef != "42" {
		t.Errorf("expected prRef '42', got %q", opts.prRef)
	}
	if opts.comment || opts.verbose || opts.dryRun || opts.toStdout {
		t.Error("flags should default to false")
	}
	if opts.rolesFlag != "" || opts.formatFlag != "" {
		t.Error("string flags should default to empty")
	}
}

func TestParseReviewArgs_AllFlags(t *testing.T) {
	opts, err := parseReviewArgs([]string{
		"42", "--comment", "--verbose", "--dry-run", "--stdout",
		"--roles", "sentinel,solver", "--format", "html",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.prRef != "42" {
		t.Errorf("expected prRef '42', got %q", opts.prRef)
	}
	if !opts.comment {
		t.Error("expected comment=true")
	}
	if !opts.verbose {
		t.Error("expected verbose=true")
	}
	if !opts.dryRun {
		t.Error("expected dryRun=true")
	}
	if !opts.toStdout {
		t.Error("expected toStdout=true")
	}
	if opts.rolesFlag != "sentinel,solver" {
		t.Errorf("expected roles 'sentinel,solver', got %q", opts.rolesFlag)
	}
	if opts.formatFlag != "html" {
		t.Errorf("expected format 'html', got %q", opts.formatFlag)
	}
}

func TestParseReviewArgs_VShorthand(t *testing.T) {
	opts, err := parseReviewArgs([]string{"42", "-v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.verbose {
		t.Error("expected -v to set verbose=true")
	}
}

func TestParseReviewArgs_RolesEquals(t *testing.T) {
	opts, err := parseReviewArgs([]string{"42", "--roles=sentinel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.rolesFlag != "sentinel" {
		t.Errorf("expected roles 'sentinel', got %q", opts.rolesFlag)
	}
}

func TestParseReviewArgs_FormatEquals(t *testing.T) {
	opts, err := parseReviewArgs([]string{"42", "--format=json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.formatFlag != "json" {
		t.Errorf("expected format 'json', got %q", opts.formatFlag)
	}
}

func TestParseReviewArgs_NoArgs(t *testing.T) {
	_, err := parseReviewArgs([]string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestParseReviewArgs_OnlyFlags(t *testing.T) {
	_, err := parseReviewArgs([]string{"--verbose", "--dry-run"})
	if err == nil {
		t.Fatal("expected error when no PR ref given")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestParseReviewArgs_UnknownFlag(t *testing.T) {
	_, err := parseReviewArgs([]string{"42", "--nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected 'unknown flag' error, got: %v", err)
	}
}

func TestParseReviewArgs_PRRefAsURL(t *testing.T) {
	opts, err := parseReviewArgs([]string{"https://github.com/org/repo/pull/42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.prRef != "https://github.com/org/repo/pull/42" {
		t.Errorf("expected full URL as prRef, got %q", opts.prRef)
	}
}

func TestParseReviewArgs_FlagsBeforePR(t *testing.T) {
	opts, err := parseReviewArgs([]string{"--format", "md", "--comment", "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.prRef != "42" {
		t.Errorf("expected prRef '42', got %q", opts.prRef)
	}
	if opts.formatFlag != "md" {
		t.Errorf("expected format 'md', got %q", opts.formatFlag)
	}
	if !opts.comment {
		t.Error("expected comment=true")
	}
}

func TestSanitizeFilename_SimpleNumber(t *testing.T) {
	if got := sanitizeFilename("42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_URLStyleRef(t *testing.T) {
	if got := sanitizeFilename("https://github.com/org/repo/pull/42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_PathTraversal(t *testing.T) {
	if got := sanitizeFilename("../../etc/passwd"); got != "passwd" {
		t.Errorf("expected 'passwd', got %q", got)
	}
}

func TestSanitizeFilename_DoubleDots(t *testing.T) {
	if got := sanitizeFilename("..42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_Tilde(t *testing.T) {
	if got := sanitizeFilename("~root"); got != "root" {
		t.Errorf("expected 'root', got %q", got)
	}
}

func TestSanitizeFilename_Backslash(t *testing.T) {
	if got := sanitizeFilename(`foo\bar`); got != "foo-bar" {
		t.Errorf("expected 'foo-bar', got %q", got)
	}
}

func TestWriteToFile_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "report.md")
	err := writeToFile("hello world", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteToFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := writeToFile("first", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeToFile("second", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	if string(data) != "second" {
		t.Errorf("expected 'second', got %q", string(data))
	}
}

func TestWriteToFile_InvalidPath(t *testing.T) {
	err := writeToFile("content", "/dev/null/impossible/path.txt")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestWriteToFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	err := writeToFile("", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(data))
	}
}

func TestExecute_Version(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prism", "version"}
	err := Execute()
	if err != nil {
		t.Errorf("expected no error for 'version', got: %v", err)
	}
}

func TestExecute_Help(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	for _, arg := range []string{"help", "--help", "-h"} {
		os.Args = []string{"prism", arg}
		err := Execute()
		if err != nil {
			t.Errorf("expected no error for %q, got: %v", arg, err)
		}
	}
}

func TestExecute_NoArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prism"}
	err := Execute()
	if err != nil {
		t.Errorf("expected no error for no args (prints usage), got: %v", err)
	}
}

func testResult() *agents.ReviewResult {
	return &agents.ReviewResult{
		Summary:  "Looks good.",
		Findings: []agents.Finding{{File: "a.go", Line: 1, Severity: "info", Summary: "ok"}},
	}
}

func testPR() *gh.PR {
	return &gh.PR{Number: "42", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}}
}

func TestOutputResults_NoFormat(t *testing.T) {
	opts := &reviewOptions{}
	err := outputResults(opts, testPR(), testResult(), nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_Markdown(t *testing.T) {
	opts := &reviewOptions{formatFlag: "md", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_MarkdownLong(t *testing.T) {
	opts := &reviewOptions{formatFlag: "markdown", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_HTML(t *testing.T) {
	opts := &reviewOptions{formatFlag: "html", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_JSON(t *testing.T) {
	opts := &reviewOptions{formatFlag: "json", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_UnknownFormat(t *testing.T) {
	opts := &reviewOptions{formatFlag: "xml"}
	err := outputResults(opts, testPR(), testResult(), nil, 0)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputResults_WritesToFile(t *testing.T) {
	// Override defaultResultsDir temporarily — we can't easily do this
	// without changing the code, so just test that toStdout=false doesn't crash.
	// The file will be written to results/ in the working dir.
	opts := &reviewOptions{formatFlag: "json", toStdout: false}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Clean up.
	_ = os.RemoveAll("results")
}

func TestRunReview_NoArgs(t *testing.T) {
	err := runReview([]string{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_UnknownFlag(t *testing.T) {
	err := runReview([]string{"42", "--bogus"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_BadRoles(t *testing.T) {
	err := runReview([]string{"42", "--roles", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for bad roles")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prism", "nonexistent"}
	err := Execute()
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' error, got: %v", err)
	}
}

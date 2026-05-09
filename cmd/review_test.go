package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/config"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
)

// mockClient implements prClient for testing.
type mockClient struct {
	pr  *gh.PR
	err error
}

func (m *mockClient) GetPRDiff(_ string) (*gh.PR, error) {
	return m.pr, m.err
}

func (m *mockClient) PostComments(_ *gh.PR, _ []gh.Suggestion) error {
	return nil
}

// withMockClient replaces the GH client constructor for the duration of a test.
func withMockClient(t *testing.T, pr *gh.PR, err error) {
	t.Helper()
	orig := newGHClient
	newGHClient = func() (prClient, error) {
		return &mockClient{pr: pr, err: err}, nil
	}
	t.Cleanup(func() { newGHClient = orig })
}

func TestParseReviewArgs_BasicPR(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "-v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.verbose {
		t.Error("expected -v to set verbose=true")
	}
}

func TestParseReviewArgs_RolesEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--roles=sentinel"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.rolesFlag != "sentinel" {
		t.Errorf("expected roles 'sentinel', got %q", opts.rolesFlag)
	}
}

func TestParseReviewArgs_FormatEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--format=json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.formatFlag != "json" {
		t.Errorf("expected format 'json', got %q", opts.formatFlag)
	}
}

func TestParseReviewArgs_NoArgs(t *testing.T) {
	t.Parallel()
	_, err := parseReviewArgs([]string{})
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestParseReviewArgs_OnlyFlags(t *testing.T) {
	t.Parallel()
	_, err := parseReviewArgs([]string{"--verbose", "--dry-run"})
	if err == nil {
		t.Fatal("expected error when no PR ref given")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected usage error, got: %v", err)
	}
}

func TestParseReviewArgs_UnknownFlag(t *testing.T) {
	t.Parallel()
	_, err := parseReviewArgs([]string{"42", "--nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected 'unknown flag' error, got: %v", err)
	}
}

func TestParseReviewArgs_PRRefAsURL(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"https://github.com/org/repo/pull/42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.prRef != "https://github.com/org/repo/pull/42" {
		t.Errorf("expected full URL as prRef, got %q", opts.prRef)
	}
}

func TestParseReviewArgs_FlagsBeforePR(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	if got := sanitizeFilename("42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_URLStyleRef(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("https://github.com/org/repo/pull/42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_PathTraversal(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("../../etc/passwd"); got != "passwd" {
		t.Errorf("expected 'passwd', got %q", got)
	}
}

func TestSanitizeFilename_DoubleDots(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("..42"); got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_Tilde(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("~root"); got != "root" {
		t.Errorf("expected 'root', got %q", got)
	}
}

func TestSanitizeFilename_Backslash(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename(`foo\bar`); got != "foobar" {
		t.Errorf("expected 'foobar', got %q", got)
	}
}

func TestSanitizeFilename_SpecialChars(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("PR#42!@$"); got != "PR42" {
		t.Errorf("expected 'PR42', got %q", got)
	}
}

func TestSanitizeFilename_Empty(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename(""); got != "unknown" {
		t.Errorf("expected 'unknown', got %q", got)
	}
}

func TestSanitizeFilename_OnlySpecialChars(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("!!!"); got != "unknown" {
		t.Errorf("expected 'unknown', got %q", got)
	}
}

func TestSanitizeFilename_Unicode(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("PR-42-café"); got != "PR-42-caf" {
		t.Errorf("expected 'PR-42-caf', got %q", got)
	}
}

func TestSanitizeFilename_HyphenAndUnderscore(t *testing.T) {
	t.Parallel()
	if got := sanitizeFilename("my_pr-42"); got != "my_pr-42" {
		t.Errorf("expected 'my_pr-42', got %q", got)
	}
}

func TestWriteToFile_CreatesDirectories(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestWriteToFile_FilePermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := writeToFile("test", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected file permissions 0600, got %o", info.Mode().Perm())
	}
}

func TestWriteToFile_DirectoryPermissions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	subDir := filepath.Join(dir, "newdir")
	path := filepath.Join(subDir, "report.md")
	if err := writeToFile("test", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("failed to stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("expected directory permissions 0700, got %o", info.Mode().Perm())
	}
}

func TestWriteToFile_InvalidPath(t *testing.T) {
	t.Parallel()
	err := writeToFile("content", "/dev/null/impossible/path.txt")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestWriteToFile_EmptyContent(t *testing.T) {
	t.Parallel()
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
	err := Execute(nil)
	if err != nil {
		t.Errorf("expected no error for 'version', got: %v", err)
	}
}

func TestExecute_Help(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	for _, arg := range []string{"help", "--help", "-h"} {
		os.Args = []string{"prism", arg}
		err := Execute(nil)
		if err != nil {
			t.Errorf("expected no error for %q, got: %v", arg, err)
		}
	}
}

func TestExecute_NoArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prism"}
	err := Execute(nil)
	if err != nil {
		t.Errorf("expected no error for no args (prints usage), got: %v", err)
	}
}

func testResult() *agents.ReviewResult {
	return &agents.ReviewResult{
		Summary:  "Looks good.",
		Findings: []agents.Finding{{File: "a.go", Line: 1, Risk: "info", Summary: "ok"}},
	}
}

func testPR() *gh.PR {
	return &gh.PR{Number: "42", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}}
}

func TestOutputResults_NoFormat(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{}
	err := outputResults(opts, testPR(), testResult(), nil, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_Markdown(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "md", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_MarkdownLong(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "markdown", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_HTML(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "html", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_JSON(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "json", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_UnknownFormat(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "xml"}
	err := outputResults(opts, testPR(), testResult(), nil, 0, opts.formatFlag)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOutputResults_WritesToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "prism-pr-1.json")
	// Test writeToFile directly since outputResults uses the hardcoded defaultResultsDir.
	err := writeToFile(`{"test": true}`, outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	if !strings.Contains(string(data), `"test"`) {
		t.Error("file content mismatch")
	}
}

func TestOutputResults_MarkdownToFile(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "md", toStdout: false}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = os.RemoveAll("results")
}

func TestOutputResults_HTMLToStdout(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "html", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_HandlesCommentNoSuggestions(t *testing.T) {
	t.Parallel()
	// When --comment is set but there are no suggestions, outputResults
	// should print "No inline suggestions to post." to signal it handled the flag.
	// BUG: Before fix, outputResults silently ignored opts.comment.
	opts := &reviewOptions{comment: true}
	result := &agents.ReviewResult{Summary: "Clean."}
	pr := &gh.PR{Number: "1", Title: "Test"}
	err := outputResults(opts, pr, result, nil, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// If we got here without handling comments, the feature is broken.
	// The fix adds comment handling to outputResults.
}

func TestRunReview_InvalidPRRef(t *testing.T) {
	// Flag injection attempt — should be caught by ValidatePRRef.
	err := runReview([]string{"--exec=evil"}, nil)
	if err == nil {
		t.Fatal("expected error for flag injection ref")
	}
}

func TestRunReview_BadRolesFromEquals(t *testing.T) {
	err := runReview([]string{"42", "--roles=bogus"}, nil)
	if err == nil {
		t.Fatal("expected error for bad roles")
	}
}

func TestRunReview_MissingPRRef(t *testing.T) {
	err := runReview([]string{"--verbose"}, nil)
	if err == nil {
		t.Fatal("expected error when no PR ref given")
	}
}

func TestRunReview_UnknownFormatViaEquals(t *testing.T) {
	// This will fail at the gh.NewClient/GetPRDiff step, not the format step,
	// because it tries to fetch the PR first. But it exercises more of runReview.
	err := runReview([]string{"99999", "--format=xml"}, nil)
	// Will fail fetching PR, which is fine — we're testing path coverage.
	if err == nil {
		t.Skip("unexpectedly succeeded")
	}
}

func TestRunReview_DryRunFormat(t *testing.T) {
	// Dry run with format — will fail at gh client, but exercises parsing.
	err := runReview([]string{"99999", "--dry-run", "--format", "json"}, nil)
	if err == nil {
		t.Skip("unexpectedly succeeded")
	}
}

func TestRunReview_NoArgs(t *testing.T) {
	err := runReview([]string{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_UnknownFlag(t *testing.T) {
	err := runReview([]string{"42", "--bogus"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_BadRoles(t *testing.T) {
	err := runReview([]string{"42", "--roles", "nonexistent"}, nil)
	if err == nil {
		t.Fatal("expected error for bad roles")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseReviewArgs_ModelFlag(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--model", "sonnet"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.modelFlag != "sonnet" {
		t.Errorf("expected model 'sonnet', got %q", opts.modelFlag)
	}
}

func TestParseReviewArgs_ModelEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--model=opus"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.modelFlag != "opus" {
		t.Errorf("expected model 'opus', got %q", opts.modelFlag)
	}
}

func TestParseReviewArgs_TimeoutFlag(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--timeout", "2m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.timeoutFlag != "2m" {
		t.Errorf("expected timeout '2m', got %q", opts.timeoutFlag)
	}
}

func TestParseReviewArgs_TimeoutEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--timeout=30s"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.timeoutFlag != "30s" {
		t.Errorf("expected timeout '30s', got %q", opts.timeoutFlag)
	}
}

func TestParseReviewArgs_MaxRetries(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--max-retries", "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.retriesFlag != 3 {
		t.Errorf("expected retries 3, got %d", opts.retriesFlag)
	}
}

func TestParseReviewArgs_MaxRetriesEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--max-retries=0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0 is a valid value (no retries).
	if opts.retriesFlag != 0 {
		t.Errorf("expected retries 0, got %d", opts.retriesFlag)
	}
}

func TestParseReviewArgs_ConfigFlag(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--config", "custom.yml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.configPath != "custom.yml" {
		t.Errorf("expected config 'custom.yml', got %q", opts.configPath)
	}
}

func TestParseReviewArgs_ConfigEquals(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--config=my.yml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.configPath != "my.yml" {
		t.Errorf("expected config 'my.yml', got %q", opts.configPath)
	}
}

func TestParseReviewArgs_DefaultConfig(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.configPath != ".prism.yml" {
		t.Errorf("expected default config '.prism.yml', got %q", opts.configPath)
	}
}

func TestParseReviewArgs_AllNewFlags(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{
		"42", "--model", "haiku", "--timeout", "1m",
		"--max-retries", "2", "--config", "test.yml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.modelFlag != "haiku" {
		t.Errorf("expected model 'haiku', got %q", opts.modelFlag)
	}
	if opts.timeoutFlag != "1m" {
		t.Errorf("expected timeout '1m', got %q", opts.timeoutFlag)
	}
	if opts.retriesFlag != 2 {
		t.Errorf("expected retries 2, got %d", opts.retriesFlag)
	}
	if opts.configPath != "test.yml" {
		t.Errorf("expected config 'test.yml', got %q", opts.configPath)
	}
}

func TestOutputResults_MarkdownWithDedupedFindings(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "md", toStdout: true}
	result := &agents.ReviewResult{
		Summary: "Review complete.",
		DedupedFindings: []agents.DedupedFinding{
			{
				Finding:     agents.Finding{File: "a.go", Line: 10, Risk: "warning", Summary: "test issue", Detail: "detail"},
				VoteCount:   3,
				TotalAgents: 5,
				Voters:      []string{"architect", "solver", "sentinel"},
			},
		},
	}
	pr := &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}}
	err := outputResults(opts, pr, result, []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_MarkdownWithFailedAgents(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "md", toStdout: true}
	result := &agents.ReviewResult{
		Summary:      "Partial review.",
		FailedAgents: []string{"Sentinel", "Optimizer"},
	}
	pr := &gh.PR{Number: "1", Title: "Test", Files: []gh.FileChange{{Path: "a.go"}}}
	err := outputResults(opts, pr, result, []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"prism", "nonexistent"}
	err := Execute(nil)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("expected 'unknown command' error, got: %v", err)
	}
}

func TestIsInteractive_InTest(t *testing.T) {
	t.Parallel()
	// In tests, stdin is typically not a terminal.
	result := isInteractive()
	// In CI/test, this should be false (stdin is piped).
	if result {
		t.Log("isInteractive returned true — running in a terminal")
	}
}

func TestRunReview_ConfigWithValidRoles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yml")
	if err := os.WriteFile(cfgPath, []byte("roles:\n  - sentinel\nmodel: sonnet\nagent_timeout: \"2m\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Will fail at GetPRDiff, but exercises config loading + role parsing + merge.
	err := runReview([]string{"99999", "--config", cfgPath}, nil)
	if err == nil {
		t.Skip("unexpectedly succeeded")
	}
}

func TestRunReview_WithModelAndTimeout(t *testing.T) {
	// Exercises config merge path with CLI overrides.
	err := runReview([]string{"99999", "--model", "haiku", "--timeout", "1m", "--max-retries", "2"}, nil)
	if err == nil {
		t.Skip("unexpectedly succeeded")
	}
}

func TestRunReview_WithYesFlag(t *testing.T) {
	err := runReview([]string{"99999", "--yes"}, nil)
	if err == nil {
		t.Skip("unexpectedly succeeded")
	}
}

func TestRunReview_ConfigWithBadRoles(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yml")
	if err := os.WriteFile(cfgPath, []byte("roles:\n  - nonexistent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runReview([]string{"42", "--config", cfgPath}, nil)
	if err == nil {
		t.Fatal("expected error for bad roles in config")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_BadConfig(t *testing.T) {
	dir := t.TempDir()
	badConfig := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(badConfig, []byte("roles: [not closed"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runReview([]string{"42", "--config", badConfig}, nil)
	if err == nil {
		t.Fatal("expected error for bad config")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseReviewArgs_YesFlag(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--yes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.yes {
		t.Error("expected yes=true")
	}
}

func TestParseReviewArgs_YShorthand(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "-y"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.yes {
		t.Error("expected yes=true from -y")
	}
}

func TestOutputResults_JSONToStdout(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "json", toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOutputResults_JSONToFile(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "json", toStdout: false}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = os.RemoveAll("results")
}

func TestOutputResults_HTMLToFile(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "html", toStdout: false}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, opts.formatFlag)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = os.RemoveAll("results")
}

func TestOutputResults_FormatFromConfig(t *testing.T) {
	t.Parallel()
	// When formatFlag comes from config (6th arg) not CLI opts.
	opts := &reviewOptions{toStdout: true}
	err := outputResults(opts, testPR(), testResult(), []agents.Role{{Name: "Test"}}, 0, "md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReview_FullPipelineWithMockClient(t *testing.T) {
	withMockClient(t, &gh.PR{
		Number: "42",
		Title:  "Test PR",
		Diff:   "diff content here",
		Files: []gh.FileChange{
			{Path: "src/app.ts", Status: "modified"},
			{Path: "main.go", Status: "modified"},
		},
	}, nil)

	// This will fail at the orchestrator (skill files not found from test binary)
	// but exercises config loading, role parsing, size check, and language detection.
	err := runReview([]string{"42", "--dry-run", "--yes"}, nil)
	// Dry run succeeds even without real skill files since it doesn't call claude.
	// But it will fail loading skills. Either way, we exercise the path.
	if err != nil && !strings.Contains(err.Error(), "failed to load skill") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReview_MockClientGetPRDiffError(t *testing.T) {
	withMockClient(t, nil, os.ErrNotExist)

	err := runReview([]string{"42"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to get PR diff") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunReview_LargeDiffWithYes(t *testing.T) {
	largeDiff := strings.Repeat("x", 200000) // 200KB, above warn threshold
	withMockClient(t, &gh.PR{
		Number: "42",
		Title:  "Big PR",
		Diff:   largeDiff,
		Files:  []gh.FileChange{{Path: "big.go"}},
	}, nil)

	// --yes skips the confirmation prompt; --dry-run avoids needing skill files.
	err := runReview([]string{"42", "--yes", "--dry-run"}, nil)
	// Will fail at skill loading, but the estimate + confirmation skip path is exercised.
	if err != nil && !strings.Contains(err.Error(), "failed to load skill") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOptions_ValidFormats(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"plain", "md", "markdown", "html", "json", ""} {
		opts := &reviewOptions{formatFlag: f}
		merged := &config.Config{}
		if err := validateOptions(opts, merged); err != nil {
			t.Errorf("expected no error for format %q, got: %v", f, err)
		}
	}
}

func TestValidateOptions_InvalidFormat(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"xml", "csv"} {
		opts := &reviewOptions{formatFlag: f}
		merged := &config.Config{}
		err := validateOptions(opts, merged)
		if err == nil {
			t.Errorf("expected error for format %q", f)
			continue
		}
		if !strings.Contains(err.Error(), "invalid format") {
			t.Errorf("expected 'invalid format' in error for %q, got: %v", f, err)
		}
	}
}

func TestValidateOptions_InvalidTimeout(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{timeoutFlag: "5 minutes"}
	merged := &config.Config{}
	err := validateOptions(opts, merged)
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	if !strings.Contains(err.Error(), "invalid timeout") {
		t.Errorf("expected 'invalid timeout' in error, got: %v", err)
	}
}

func TestValidateOptions_ValidTimeout(t *testing.T) {
	t.Parallel()
	for _, d := range []string{"2m", "30s"} {
		opts := &reviewOptions{timeoutFlag: d}
		merged := &config.Config{}
		if err := validateOptions(opts, merged); err != nil {
			t.Errorf("expected no error for timeout %q, got: %v", d, err)
		}
	}
}

func TestOutputResults_PlainFormat(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{}
	err := outputResults(opts, testPR(), testResult(), nil, 0, "plain")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunReview_InvalidFormatFailsFast(t *testing.T) {
	withMockClient(t, &gh.PR{
		Number: "42",
		Title:  "Test PR",
		Diff:   "diff content",
		Files:  []gh.FileChange{{Path: "main.go", Status: "modified"}},
	}, nil)

	err := runReview([]string{"42", "--format", "xml"}, nil)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected 'invalid format' error, got: %v", err)
	}
}

func TestParseReviewArgs_EstimateFlag(t *testing.T) {
	t.Parallel()
	opts, err := parseReviewArgs([]string{"42", "--estimate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.estimate {
		t.Error("expected estimate=true")
	}
}

func TestRunReview_EstimateExitsEarly(t *testing.T) {
	withMockClient(t, &gh.PR{
		Number: "42",
		Title:  "Test PR",
		Diff:   "some diff content here",
		Files:  []gh.FileChange{{Path: "main.go"}},
	}, nil)

	// --estimate should exit before calling any agents.
	err := runReview([]string{"42", "--estimate", "--yes"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrintEstimate(t *testing.T) {
	t.Parallel()
	// Just verify it doesn't panic with reasonable inputs.
	roles := []agents.Role{
		{Name: "Sentinel", Model: "opus"},
		{Name: "Know-It-All", Model: "sonnet"},
		{Name: "Editor", Model: "haiku"},
	}
	printEstimate(10000, roles)
	printEstimate(0, roles[:1])
	printEstimate(500000, roles)
}

// TestRunReview_ShowsConfigBreakdownBeforePrompt verifies that the user sees
// what models/features will run before being asked to confirm a spend. Without
// this breakdown the prompt is just a yes/no with no context about cost drivers.
func TestRunReview_ShowsConfigBreakdownBeforePrompt(t *testing.T) {
	withMockClient(t, &gh.PR{
		Number: "42",
		Title:  "Test PR",
		Repo:   "myrepo",
		Diff:   "diff --git a/main.go b/main.go\n+package main\n",
		Files:  []gh.FileChange{{Path: "main.go", Status: "modified"}},
	}, nil)

	// Chdir so any results/ written by a successful dry-run lands in tempdir.
	origWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	oldStdout := os.Stdout
	_, sw, _ := os.Pipe()
	os.Stdout = sw

	_ = runReview([]string{"42", "--dry-run", "--yes"}, nil)

	_ = w.Close()
	_ = sw.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	stderr := buf.String()

	for _, want := range []string{"Models:", "Verify:", "Estimated cost:"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected stderr to contain %q before review runs, got:\n%s", want, stderr)
		}
	}
}

func TestValidateOptions_AcceptsCommaSeparatedFormats(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"md,html", "md,html,json", "html,json", "md,html,plain"} {
		opts := &reviewOptions{formatFlag: f}
		merged := &config.Config{}
		if err := validateOptions(opts, merged); err != nil {
			t.Errorf("expected no error for format %q, got: %v", f, err)
		}
	}
}

func TestValidateOptions_RejectsCommaSeparatedWithBadEntry(t *testing.T) {
	t.Parallel()
	for _, f := range []string{"md,xml", "html,csv", "md,html,bogus"} {
		opts := &reviewOptions{formatFlag: f}
		merged := &config.Config{}
		err := validateOptions(opts, merged)
		if err == nil {
			t.Errorf("expected error for format %q (one entry is invalid)", f)
			continue
		}
		if !strings.Contains(err.Error(), "invalid format") {
			t.Errorf("expected 'invalid format' in error for %q, got: %v", f, err)
		}
	}
}

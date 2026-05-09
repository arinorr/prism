package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/config"
	"github.com/arinorr/prism/internal/gh"
)

func TestProgress_WritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	progress("hello %s\n", "world")
	progressln("line two")

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	got := buf.String()

	if !strings.Contains(got, "hello world") {
		t.Errorf("progress() should write to stderr, got %q", got)
	}
	if !strings.Contains(got, "line two") {
		t.Errorf("progressln() should write to stderr, got %q", got)
	}
}

func TestProgress_NothingOnStdout(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	oldStderr := os.Stderr
	_, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	progress("this goes to stderr\n")

	_ = w.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if buf.String() != "" {
		t.Errorf("progress() should not write to stdout, got %q", buf.String())
	}
}

func TestRunReview_StdoutStderrSeparation(t *testing.T) {
	pr := &gh.PR{
		Number: "42",
		Repo:   "test-repo",
		Title:  "Test PR",
		Body:   "Description",
		Diff:   "diff --git a/main.go b/main.go\n+package main\n",
		Files:  []gh.FileChange{{Path: "main.go", Status: "added"}},
	}
	withMockClient(t, pr, nil)

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW

	_ = runReview([]string{"42", "--dry-run", "--format", "json", "--stdout"}, nil)

	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdoutBuf.ReadFrom(stdoutR)
	_, _ = stderrBuf.ReadFrom(stderrR)

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if !strings.Contains(stderr, "DRY RUN") && !strings.Contains(stderr, "Reviewing") {
		t.Errorf("expected progress on stderr, got %q", stderr)
	}

	if strings.Contains(stdout, "🔍") {
		t.Errorf("stdout should not contain progress emojis, got %q", stdout[:min(len(stdout), 200)])
	}
}

// TestOutputPath_VaultStyleTimestamp verifies the vault-style filename
// includes YYMMDD-HHMMSS — reruns within the same minute can't collide.
func TestOutputPath_VaultStyleTimestamp(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 8, 12, 34, 56, 0, time.UTC)
	got := outputPath("myrepo", "42", "md", now)
	want := filepath.Join("results", "myrepo-pr-42", "260508-123456-myrepo-pr-42.md")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestOutputPath_PadsZeros(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := outputPath("repo", "1", "html", now)
	want := filepath.Join("results", "repo-pr-1", "260101-000000-repo-pr-1.html")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestOutputPath_TwoRunsSameMinuteDontCollide guards the user-reported bug:
// two reviews started within 60s of each other previously overwrote each
// other's reports. Second-resolution timestamps prevent that.
func TestOutputPath_TwoRunsSameMinuteDontCollide(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 5, 8, 12, 34, 10, 0, time.UTC)
	t2 := time.Date(2026, 5, 8, 12, 34, 50, 0, time.UTC)
	if outputPath("repo", "1", "md", t1) == outputPath("repo", "1", "md", t2) {
		t.Error("paths within the same minute must differ at second resolution")
	}
}

// TestOutputPath_OwnerRepoDisambiguation: passing "owner/repo" preserves both
// halves (slash → "-") so two repos with the same short name don't collide
// under results/. Without this, sanitizeFilename's URL-path handling would
// strip everything before the last "/", leaving just the bare repo name.
func TestOutputPath_OwnerRepoDisambiguation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 8, 12, 34, 56, 0, time.UTC)
	got := outputPath("arinorr/prism", "42", "md", now)
	want := filepath.Join("results", "arinorr-prism-pr-42", "260508-123456-arinorr-prism-pr-42.md")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestPrRepoIdent_FallsBackToRepo guards a regression we shipped: the
// git-only client path (no `gh` CLI installed) populates pr.Repo but leaves
// pr.OwnerRepo empty. Without the fallback, those reports went to
// results/unknown-pr-42/ instead of using the detected repo name.
func TestPrRepoIdent_FallsBackToRepo(t *testing.T) {
	t.Parallel()
	withOwner := &gh.PR{Repo: "myrepo", OwnerRepo: "owner/myrepo"}
	if got := prRepoIdent(withOwner); got != "owner/myrepo" {
		t.Errorf("with OwnerRepo set: expected 'owner/myrepo', got %q", got)
	}
	gitOnly := &gh.PR{Repo: "myrepo"} // OwnerRepo unset (git-only fallback)
	if got := prRepoIdent(gitOnly); got != "myrepo" {
		t.Errorf("git-only path: expected 'myrepo' fallback, got %q", got)
	}
}

// TestConfigDefault_IncludesMarkdownAndHTML pins the default format to write
// both markdown and HTML files. Generating both is free (no extra API calls,
// just a second render pass over the same Data struct), and users get both
// the editable source-of-truth (md) and the polished view (html).
func TestConfigDefault_IncludesMarkdownAndHTML(t *testing.T) {
	t.Parallel()
	def := config.Default()
	if def.Format != "md,html" {
		t.Errorf("default format should be 'md,html', got %q", def.Format)
	}
}

// TestOutputResults_WritesAllRequestedFormats checks the comma-separated
// --format md,html,json case writes one file per requested format.
// Not t.Parallel() because os.Chdir mutates process-global state.
func TestOutputResults_WritesAllRequestedFormats(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	pr := &gh.PR{Number: "42", Title: "Test", Repo: "myrepo", OwnerRepo: "owner/myrepo", Files: []gh.FileChange{{Path: "a.go"}}}
	opts := &reviewOptions{formatFlag: "md,html,json"}
	if err := outputResults(opts, pr, testResult(), []agents.Role{{Name: "Test"}}, 0, "md,html,json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join("results", "owner-myrepo-pr-42"))
	if err != nil {
		t.Fatalf("failed to read results dir: %v", err)
	}
	gotExts := map[string]bool{}
	for _, e := range entries {
		if i := strings.LastIndex(e.Name(), "."); i >= 0 {
			gotExts[e.Name()[i+1:]] = true
		}
	}
	for _, ext := range []string{"md", "html", "json"} {
		if !gotExts[ext] {
			t.Errorf("expected a .%s file in results dir, got entries: %v", ext, entries)
		}
	}
}

// TestValidateOptions_StdoutWithMultiFormatErrors checks that --stdout +
// multi-format fails fast in validateOptions (which runs before any GitHub
// fetch or LLM call), not in outputResults (which would burn tokens first).
func TestValidateOptions_StdoutWithMultiFormatErrors(t *testing.T) {
	t.Parallel()
	opts := &reviewOptions{formatFlag: "md,html", toStdout: true}
	merged := &config.Config{}
	err := validateOptions(opts, merged)
	if err == nil {
		t.Fatal("expected error for --stdout with multiple formats")
	}
	if !strings.Contains(err.Error(), "stdout") {
		t.Errorf("expected error mentioning stdout, got: %v", err)
	}
}

// TestRunReview_StdoutMultiFormatFailsFast verifies that the --stdout +
// multi-format error fires before fetchPR runs, so it never reaches the
// orchestrator. The mock client tracks whether it was called.
func TestRunReview_StdoutMultiFormatFailsFast(t *testing.T) {
	called := false
	orig := newGHClient
	newGHClient = func() (prClient, error) {
		called = true
		return &mockClient{pr: &gh.PR{Number: "42"}}, nil
	}
	t.Cleanup(func() { newGHClient = orig })

	err := runReview([]string{"42", "--stdout", "--format", "md,html"}, nil)
	if err == nil {
		t.Fatal("expected error for --stdout with multiple formats")
	}
	if !strings.Contains(err.Error(), "stdout") {
		t.Errorf("expected stdout-related error, got: %v", err)
	}
	if called {
		t.Error("validateOptions should fail before GH client is constructed")
	}
}

// TestOutputResults_PlainInMultiFormatWritesToFile verifies that "plain"
// inside a comma-separated format list writes to a .txt file like other
// formats, instead of always going to stdout.
func TestOutputResults_PlainInMultiFormatWritesToFile(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}

	pr := &gh.PR{Number: "42", Title: "Test", Repo: "myrepo", OwnerRepo: "owner/myrepo", Files: []gh.FileChange{{Path: "a.go"}}}
	opts := &reviewOptions{formatFlag: "md,plain"}
	if err := outputResults(opts, pr, testResult(), []agents.Role{{Name: "Test"}}, 0, "md,plain"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join("results", "owner-myrepo-pr-42"))
	if err != nil {
		t.Fatalf("failed to read results dir: %v", err)
	}
	gotExts := map[string]bool{}
	for _, e := range entries {
		if i := strings.LastIndex(e.Name(), "."); i >= 0 {
			gotExts[e.Name()[i+1:]] = true
		}
	}
	for _, ext := range []string{"md", "txt"} {
		if !gotExts[ext] {
			t.Errorf("expected a .%s file in results dir, got entries: %v", ext, entries)
		}
	}
}

func TestRunReview_DefaultQuieterThanVerbose(t *testing.T) {
	pr := &gh.PR{
		Number: "42",
		Repo:   "test-repo",
		Title:  "Test PR",
		Diff:   "diff --git a/main.go b/main.go\n+package main\n",
		Files:  []gh.FileChange{{Path: "main.go", Status: "added"}},
	}
	withMockClient(t, pr, nil)

	oldStderr := os.Stderr
	r1, w1, _ := os.Pipe()
	os.Stderr = w1
	oldStdout := os.Stdout
	_, sw1, _ := os.Pipe()
	os.Stdout = sw1

	_ = runReview([]string{"42", "--dry-run"}, nil)

	_ = w1.Close()
	_ = sw1.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var buf1 bytes.Buffer
	_, _ = buf1.ReadFrom(r1)
	defaultOutput := buf1.String()

	withMockClient(t, pr, nil)
	r2, w2, _ := os.Pipe()
	os.Stderr = w2
	_, sw2, _ := os.Pipe()
	os.Stdout = sw2

	_ = runReview([]string{"42", "--dry-run", "--verbose"}, nil)

	_ = w2.Close()
	_ = sw2.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var buf2 bytes.Buffer
	_, _ = buf2.ReadFrom(r2)
	verboseOutput := buf2.String()

	if len(verboseOutput) <= len(defaultOutput) {
		t.Errorf("verbose (%d bytes) should be longer than default (%d bytes)",
			len(verboseOutput), len(defaultOutput))
	}

	if strings.Contains(defaultOutput, "Token estimate") {
		t.Error("default mode should NOT show token estimate")
	}

	if !strings.Contains(verboseOutput, "Token estimate") {
		t.Error("verbose mode should show token estimate")
	}
}

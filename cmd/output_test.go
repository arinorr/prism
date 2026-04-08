package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestProgress_WritesToStderr(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	progress("hello %s\n", "world")
	progressln("line two")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
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

	w.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)

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

	_ = runReview([]string{"42", "--dry-run", "--format", "json", "--stdout"})

	stdoutW.Close()
	stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutBuf.ReadFrom(stdoutR)
	stderrBuf.ReadFrom(stderrR)

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	// Stderr should have progress.
	if !strings.Contains(stderr, "DRY RUN") && !strings.Contains(stderr, "Reviewing") {
		t.Errorf("expected progress on stderr, got %q", stderr)
	}

	// Stdout should NOT have progress emojis.
	if strings.Contains(stdout, "🔍") {
		t.Errorf("stdout should not contain progress emojis, got %q", stdout[:min(len(stdout), 200)])
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

	// Run default (non-verbose).
	oldStderr := os.Stderr
	r1, w1, _ := os.Pipe()
	os.Stderr = w1
	oldStdout := os.Stdout
	_, sw1, _ := os.Pipe()
	os.Stdout = sw1

	_ = runReview([]string{"42", "--dry-run"})

	w1.Close()
	sw1.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var buf1 bytes.Buffer
	buf1.ReadFrom(r1)
	defaultOutput := buf1.String()

	// Run verbose.
	withMockClient(t, pr, nil)
	r2, w2, _ := os.Pipe()
	os.Stderr = w2
	_, sw2, _ := os.Pipe()
	os.Stdout = sw2

	_ = runReview([]string{"42", "--dry-run", "--verbose"})

	w2.Close()
	sw2.Close()
	os.Stderr = oldStderr
	os.Stdout = oldStdout

	var buf2 bytes.Buffer
	buf2.ReadFrom(r2)
	verboseOutput := buf2.String()

	// Verbose should produce more output than default.
	if len(verboseOutput) <= len(defaultOutput) {
		t.Errorf("verbose (%d bytes) should be longer than default (%d bytes)",
			len(verboseOutput), len(defaultOutput))
	}

	// Default should NOT show token estimate.
	if strings.Contains(defaultOutput, "Token estimate") {
		t.Error("default mode should NOT show token estimate")
	}

	// Verbose SHOULD show token estimate.
	if !strings.Contains(verboseOutput, "Token estimate") {
		t.Error("verbose mode should show token estimate")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package agents

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/index"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
	"github.com/arinorr/prism/internal/resolve"
)

// TestIntegration_ReviewWithVerification exercises the full pipeline with
// verification enabled, using a mock LLM that:
// - Returns findings from agents.
// - Dismisses one finding via Haiku.
// - Confirms others.
func TestIntegration_ReviewWithVerification(t *testing.T) {
	t.Parallel()

	// Set up a temp repo with source files the verifier can index.
	repoDir := t.TempDir()
	writeRepoFile(t, repoDir, "handler.go", `package main

import "fmt"

func HandleRequest(data string) error {
	if data == "" {
		return fmt.Errorf("empty data")
	}
	return Process(data)
}

func Process(data string) error {
	if len(data) > 1000 {
		return fmt.Errorf("data too large")
	}
	return nil
}
`)

	// Agent mock: returns two findings.
	agentFindings := `{"findings": [
		{
			"file": "handler.go",
			"line": 5,
			"risk": "warning",
			"category": "bug",
			"scope": "changed",
			"confidence": 0.8,
			"summary": "data parameter could be nil",
			"detail": "The data parameter is not checked for nil before use."
		},
		{
			"file": "handler.go",
			"line": 9,
			"risk": "warning",
			"category": "bug",
			"scope": "changed",
			"confidence": 0.7,
			"summary": "Process return value not validated",
			"detail": "The return value from Process should be checked for specific error types."
		}
	]}`

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			// Agent dispatch: return findings.
			if strings.Contains(req.UserPrompt, "<pr-diff>") {
				return agentFindings, llm.Usage{CostUSD: 0.01, InputTokens: 1000, OutputTokens: 200}, nil
			}
			// Haiku verification: dismiss the nil check (data is string, not pointer).
			if req.Model == ModelTierFast {
				return "1. NO — data is a string type, cannot be nil in Go\n2. YES — confirmed\n",
					llm.Usage{CostUSD: 0.001}, nil
			}
			// Opus should not be called (no UNSURE findings).
			return `{"verdict": "confirmed", "reason": "ok"}`, llm.Usage{CostUSD: 0.01}, nil
		},
	}

	roles := testRoles()
	orch := &Orchestrator{
		roles: roles,
		opts: &Options{
			Out:           io.Discard,
			ErrOut:        io.Discard,
			Verify:        true,
			RepoRoot:      repoDir,
			Languages:     []string{"go"},
			ExplicitRoles: true,
		},
		skills: testSkills(),
		llm:    mock,
	}

	pr := &gh.PR{
		Number: "99",
		Title:  "Add request handling",
		Body:   "Implements basic request handler",
		Diff:   "diff --git a/handler.go b/handler.go\n+func HandleRequest(data string) error {\n",
		Files:  []gh.FileChange{{Path: "handler.go", Status: "added"}},
	}

	result, err := orch.Review(context.Background(), pr)
	if err != nil {
		t.Fatal(err)
	}

	// One finding should be dismissed (nil check on string type).
	if result.DismissedCount != 1 {
		t.Errorf("DismissedCount = %d, want 1", result.DismissedCount)
	}

	// One finding should remain.
	remaining := len(result.DedupedFindings)
	if remaining == 0 {
		t.Error("expected at least one finding to survive verification")
	}

	// Verifier usage should be non-zero.
	if result.VerifierUsage.CostUSD == 0 {
		t.Error("expected non-zero verifier cost")
	}

	// Total usage should include verifier usage.
	if result.Usage.CostUSD <= result.VerifierUsage.CostUSD {
		// Agent cost should be > verifier cost.
		t.Logf("total cost: $%.4f, verifier cost: $%.4f", result.Usage.CostUSD, result.VerifierUsage.CostUSD)
	}

	// Health score should be computed from verified findings only.
	if result.HealthScore.Score == 0 && remaining > 0 {
		t.Error("health score should be recomputed after verification")
	}
}

// TestIntegration_ReviewWithoutVerification ensures the pipeline works
// unchanged when verification is disabled (default).
func TestIntegration_ReviewWithoutVerification(t *testing.T) {
	t.Parallel()

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			return `{"findings": [{"file": "a.go", "line": 1, "risk": "info", "category": "style", "scope": "changed", "confidence": 0.8, "summary": "minor style"}]}`,
				llm.Usage{CostUSD: 0.01}, nil
		},
	}

	orch := &Orchestrator{
		roles:  testRoles(),
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard, Verify: false, ExplicitRoles: true},
		skills: testSkills(),
		llm:    mock,
	}

	pr := &gh.PR{
		Number: "1",
		Title:  "Test",
		Diff:   "diff --git a/a.go b/a.go\n+test\n",
		Files:  []gh.FileChange{{Path: "a.go"}},
	}

	result, err := orch.Review(context.Background(), pr)
	if err != nil {
		t.Fatal(err)
	}

	// No verification should have happened.
	if result.DismissedCount != 0 {
		t.Errorf("DismissedCount = %d, want 0", result.DismissedCount)
	}
	if result.VerifierUsage.CostUSD != 0 {
		t.Errorf("verifier cost = $%.4f, want 0", result.VerifierUsage.CostUSD)
	}
	if result.VerifierError != "" {
		t.Errorf("unexpected verifier error: %s", result.VerifierError)
	}
}

// TestIntegration_VerificationLLMFailureGraceful ensures that when the verifier's
// LLM calls fail, findings pass through as confirmed (safe fallback) rather
// than causing the review to fail.
func TestIntegration_VerificationLLMFailureGraceful(t *testing.T) {
	t.Parallel()

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if strings.Contains(req.UserPrompt, "<pr-diff>") {
				return `{"findings": [{"file": "a.go", "line": 1, "risk": "warning", "category": "bug", "scope": "changed", "confidence": 0.9, "summary": "issue"}]}`,
					llm.Usage{CostUSD: 0.01}, nil
			}
			// Verifier LLM calls always fail.
			return "", llm.Usage{}, context.DeadlineExceeded
		},
	}

	repoDir := t.TempDir()
	writeRepoFile(t, repoDir, "a.go", "package main\nfunc A() {}\n")

	orch := &Orchestrator{
		roles: testRoles(),
		opts: &Options{
			Out:           io.Discard,
			ErrOut:        io.Discard,
			Verify:        true,
			RepoRoot:      repoDir,
			Languages:     []string{"go"},
			ExplicitRoles: true,
		},
		skills: testSkills(),
		llm:    mock,
	}

	pr := &gh.PR{
		Number: "2",
		Title:  "Test",
		Diff:   "diff --git a/a.go b/a.go\n+func A() {}\n",
		Files:  []gh.FileChange{{Path: "a.go"}},
	}

	result, err := orch.Review(context.Background(), pr)
	if err != nil {
		t.Fatal(err)
	}

	// Review should succeed — findings pass through as confirmed (safe fallback).
	if len(result.DedupedFindings) == 0 {
		t.Error("expected findings to survive LLM failure (confirmed fallback)")
	}

	// Findings should be confirmed despite LLM failures.
	for _, f := range result.DedupedFindings {
		if f.VerificationStatus != StatusConfirmed && f.VerificationStatus != "" {
			t.Errorf("expected confirmed or empty status on LLM failure, got %s", f.VerificationStatus)
		}
	}
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// verifyIndexBuilds tests that Build() works with the repoDir used in integration tests.
func TestIntegration_IndexBuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeRepoFile(t, dir, "main.go", "package main\n\nfunc Main() {}\n")
	writeRepoFile(t, dir, "lib/util.ts", "export function helper() { return 1; }\n")

	idx, err := index.Build(context.Background(), dir, []string{"go", "typescript"})
	if err != nil {
		t.Fatal(err)
	}

	if idx.Size() < 2 {
		t.Errorf("expected at least 2 symbols, got %d", idx.Size())
	}

	// Verify the resolver can use this index.
	r := resolve.NewResolver(idx, dir)
	r.PreloadFiles([]string{"main.go"})

	rc, err := r.Resolve("main.go", 3)
	if err != nil {
		t.Fatal(err)
	}

	if rc.EnclosingScope == nil {
		t.Error("expected enclosing scope for Main()")
	}
}

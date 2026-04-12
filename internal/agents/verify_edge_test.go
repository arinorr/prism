package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arinorr/prism/internal/parse"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
	"github.com/arinorr/prism/internal/resolve"
)

func TestVerify_BatchesByFile(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	var haikuCalls int32
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				atomic.AddInt32(&haikuCalls, 1)
				// Count findings in prompt by looking for "## Finding" markers.
				count := strings.Count(req.UserPrompt, "## Finding")
				var lines []string
				for i := 1; i <= count; i++ {
					lines = append(lines, fmt.Sprintf("%d. YES — confirmed", i))
				}
				return strings.Join(lines, "\n"), llm.Usage{CostUSD: 0.001}, nil
			}
			return `{"verdict": "confirmed", "reason": "ok"}`, llm.Usage{}, nil
		},
	}

	// Create findings that should be batched (same file).
	findings := make([]DedupedFinding, 8)
	for i := range findings {
		findings[i] = DedupedFinding{
			Finding: Finding{
				File:     "handler.go",
				Line:     i + 1,
				Risk:     RiskWarning,
				Category: "bug",
				Summary:  fmt.Sprintf("issue %d", i),
			},
			VoteCount:   1,
			TotalAgents: 7,
		}
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 8 {
		t.Errorf("expected 8 findings, got %d", len(result))
	}

	// 8 findings, maxFindingsPerCall=5, so should be 2 Haiku calls.
	calls := atomic.LoadInt32(&haikuCalls)
	if calls != 2 {
		t.Errorf("expected 2 Haiku batch calls, got %d", calls)
	}
}

func TestVerify_MultipleFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestFile(t, dir, "a.go", `package main
func A() error { return nil }
`)
	writeTestFile(t, dir, "b.go", `package main
func B() error { return nil }
`)

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	for _, f := range []string{"a.go", "b.go"} {
		src, _ := os.ReadFile(filepath.Join(dir, f))
		for _, sym := range scanner.Scan(f, src) {
			idx.Add(sym)
		}
	}
	idx.Freeze()

	r := resolve.NewResolver(idx, dir)
	r.PreloadFiles([]string{"a.go", "b.go"})

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			return "1. YES — confirmed\n", llm.Usage{CostUSD: 0.001}, nil
		},
	}

	findings := []DedupedFinding{
		{Finding: Finding{File: "a.go", Line: 2, Risk: RiskWarning, Summary: "issue in a"}, VoteCount: 1, TotalAgents: 7},
		{Finding: Finding{File: "b.go", Line: 2, Risk: RiskWarning, Summary: "issue in b"}, VoteCount: 1, TotalAgents: 7},
	}

	v := NewVerifier(mock, r, &Options{})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result))
	}
}

func TestVerify_BudgetStopsEarly(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	var calls int32
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			atomic.AddInt32(&calls, 1)
			return "1. YES — confirmed\n", llm.Usage{CostUSD: 1.00}, nil // expensive
		},
	}

	findings := make([]DedupedFinding, 10)
	for i := range findings {
		findings[i] = DedupedFinding{
			Finding:   Finding{File: "handler.go", Line: i + 1, Risk: RiskWarning, Summary: fmt.Sprintf("issue %d", i)},
			VoteCount: 1, TotalAgents: 7,
		}
	}

	v := NewVerifier(mock, resolver, &Options{VerifierBudgetUSD: 0.50})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	// Should have some unverified findings due to budget.
	hasUnverified := false
	for _, f := range result {
		if f.VerificationStatus == StatusUnverified {
			hasUnverified = true
			break
		}
	}
	if !hasUnverified {
		t.Error("expected some findings to be unverified due to budget")
	}

	// All findings should still come through (unverified pass through).
	if len(result) != 10 {
		t.Errorf("expected all 10 findings (some unverified), got %d", len(result))
	}
}

func TestVerify_OpusParseFailure(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. UNSURE — need more info\n", llm.Usage{CostUSD: 0.001}, nil
			}
			// Opus returns garbage.
			return "I'm not sure what to do with this", llm.Usage{CostUSD: 0.01}, nil
		},
	}

	findings := []DedupedFinding{
		{Finding: Finding{File: "handler.go", Line: 3, Risk: RiskWarning, Summary: "test"}, VoteCount: 1, TotalAgents: 7},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	// Parse failure → confirmed (safe fallback).
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].VerificationStatus != StatusConfirmed {
		t.Errorf("expected confirmed on parse failure, got %s", result[0].VerificationStatus)
	}
}

func TestVerify_OpusError(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. UNSURE — unclear\n", llm.Usage{}, nil
			}
			return "", llm.Usage{}, fmt.Errorf("opus rate limited")
		},
	}

	findings := []DedupedFinding{
		{Finding: Finding{File: "handler.go", Line: 3, Risk: RiskCritical, Summary: "critical bug"}, VoteCount: 5, TotalAgents: 7},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	// Opus error → confirmed (safe fallback).
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].VerificationStatus != StatusConfirmed {
		t.Errorf("expected confirmed on Opus error, got %s", result[0].VerificationStatus)
	}
}

func TestVerify_FindingWithNoFile(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			return "1. YES — confirmed\n", llm.Usage{}, nil
		},
	}

	findings := []DedupedFinding{
		{Finding: Finding{File: "", Line: 0, Risk: RiskInfo, Summary: "general note"}, VoteCount: 1, TotalAgents: 7},
		{Finding: Finding{File: "handler.go", Line: 3, Risk: RiskWarning, Summary: "real issue"}, VoteCount: 2, TotalAgents: 7},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	// Both should come through.
	if len(result) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result))
	}
}

func TestVerify_VerifierModelOverride(t *testing.T) {
	t.Parallel()
	resolver := testResolver(t)

	var opusModel string
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. UNSURE — unclear\n", llm.Usage{}, nil
			}
			opusModel = req.Model
			return `{"verdict": "confirmed", "reason": "real"}`, llm.Usage{}, nil
		},
	}

	findings := []DedupedFinding{
		{Finding: Finding{File: "handler.go", Line: 3, Risk: RiskWarning, Summary: "test"}, VoteCount: 1, TotalAgents: 7},
	}

	v := NewVerifier(mock, resolver, &Options{VerifierModel: "sonnet"})
	_, _, err := v.Verify(context.Background(), findings)
	if err != nil {
		t.Fatal(err)
	}

	if opusModel != "sonnet" {
		t.Errorf("expected verifier model override 'sonnet', got %q", opusModel)
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `{"verdict": "confirmed"}`, `{"verdict": "confirmed"}`},
		{"code fence", "```json\n{\"verdict\": \"dismissed\"}\n```", `{"verdict": "dismissed"}`},
		{"bare fence", "```\n{\"verdict\": \"confirmed\"}\n```", `{"verdict": "confirmed"}`},
		{"text around", "Here is my analysis:\n{\"verdict\": \"confirmed\"}\nDone.", `{"verdict": "confirmed"}`},
		{"no json", "just text", "just text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

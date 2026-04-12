package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/arinorr/prism/internal/parse"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
	"github.com/arinorr/prism/internal/resolve"
)

func testResolver(t *testing.T) *resolve.Resolver {
	t.Helper()
	dir := t.TempDir()

	src := `package main

func HandleRequest(data any) error {
	if data == nil {
		return fmt.Errorf("nil data")
	}
	return process(data)
}

func process(data any) error {
	return nil
}
`
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	syms := scanner.Scan("handler.go", []byte(src))
	for _, s := range syms {
		idx.Add(s)
	}
	idx.Freeze()

	r := resolve.NewResolver(idx, dir)
	r.PreloadFiles([]string{"handler.go"})
	return r
}

func testFindings() []DedupedFinding {
	return []DedupedFinding{
		{
			Finding: Finding{
				File:     "handler.go",
				Line:     3,
				Risk:     RiskWarning,
				Category: "bug",
				Summary:  "data parameter is untyped any, could be nil",
				Detail:   "The data parameter is typed as any and could be nil at runtime.",
			},
			VoteCount:   3,
			TotalAgents: 7,
			Voters:      []string{"solver", "know-it-all", "architect"},
		},
		{
			Finding: Finding{
				File:     "handler.go",
				Line:     7,
				Risk:     RiskCritical,
				Category: "bug",
				Summary:  "process() return value not checked",
				Detail:   "The return value of process() is returned but may contain an error.",
			},
			VoteCount:   2,
			TotalAgents: 7,
			Voters:      []string{"solver", "sentinel"},
		},
	}
}

func TestVerify_AllConfirmed(t *testing.T) {
	resolver := testResolver(t)
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. YES — the issue exists\n2. YES — confirmed\n", llm.Usage{CostUSD: 0.001}, nil
			}
			// Opus should not be called.
			t.Error("unexpected Opus call")
			return "", llm.Usage{}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, usage, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result))
	}
	if usage.CostUSD == 0 {
		t.Error("expected non-zero usage")
	}
	for _, f := range result {
		if f.VerificationStatus != StatusConfirmed {
			t.Errorf("expected confirmed, got %s", f.VerificationStatus)
		}
	}
}

func TestVerify_HaikuDismisses(t *testing.T) {
	resolver := testResolver(t)
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. NO — nil check exists on line 4\n2. YES — confirmed\n", llm.Usage{CostUSD: 0.001}, nil
			}
			t.Error("unexpected Opus call for dismissed finding")
			return "", llm.Usage{}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 finding (one dismissed), got %d", len(result))
	}
	if result[0].Summary != "process() return value not checked" {
		t.Errorf("wrong finding survived: %s", result[0].Summary)
	}
}

func TestVerify_UnsureEscalatesToOpus(t *testing.T) {
	resolver := testResolver(t)
	opusCalled := false
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. UNSURE — need more context\n2. YES — confirmed\n", llm.Usage{CostUSD: 0.001}, nil
			}
			opusCalled = true
			return `{"verdict": "dismissed", "reason": "nil check on line 4 handles this"}`, llm.Usage{CostUSD: 0.01}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	if !opusCalled {
		t.Error("expected Opus to be called for UNSURE finding")
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 finding (one dismissed by Opus), got %d", len(result))
	}
}

func TestVerify_OpusDowngrades(t *testing.T) {
	resolver := testResolver(t)
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "1. UNSURE — unclear\n2. YES — confirmed\n", llm.Usage{CostUSD: 0.001}, nil
			}
			return `{"verdict": "downgraded", "reason": "real but minor", "adjusted_risk": "info"}`, llm.Usage{CostUSD: 0.01}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}

	// First finding should be downgraded to info.
	if result[0].VerificationStatus != StatusDowngraded {
		t.Errorf("expected downgraded, got %s", result[0].VerificationStatus)
	}
	if result[0].Risk != RiskInfo {
		t.Errorf("expected risk=info, got %s", result[0].Risk)
	}
}

func TestVerify_BudgetExceeded(t *testing.T) {
	resolver := testResolver(t)
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			return "1. YES — confirmed\n2. YES — confirmed\n", llm.Usage{CostUSD: 0.50}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{VerifierBudgetUSD: 0.01})
	result, _, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	// All findings should come through (first batch uses budget, rest unverified).
	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}
}

func TestVerify_EmptyFindings(t *testing.T) {
	resolver := testResolver(t)
	mock := &llmtest.Mock{}

	v := NewVerifier(mock, resolver, &Options{})
	result, usage, err := v.Verify(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result))
	}
	if usage.CostUSD != 0 {
		t.Error("expected zero cost for empty findings")
	}
}

func TestVerify_HaikuFailureFallsThrough(t *testing.T) {
	resolver := testResolver(t)
	opusCalls := 0
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			if req.Model == ModelTierFast {
				return "", llm.Usage{}, fmt.Errorf("haiku unavailable")
			}
			opusCalls++
			return `{"verdict": "confirmed", "reason": "real issue"}`, llm.Usage{CostUSD: 0.01}, nil
		},
	}

	v := NewVerifier(mock, resolver, &Options{})
	result, _, err := v.Verify(context.Background(), testFindings())
	if err != nil {
		t.Fatal(err)
	}

	// All findings should pass through (Haiku failed, Opus confirms).
	if len(result) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result))
	}
	if opusCalls == 0 {
		t.Error("expected Opus to be called as fallback")
	}
}

func TestParseHaikuResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		count    int
		want     []VerificationStatus
	}{
		{
			name:     "all yes",
			response: "1. YES — confirmed\n2. YES — looks real\n",
			count:    2,
			want:     []VerificationStatus{StatusConfirmed, StatusConfirmed},
		},
		{
			name:     "mixed",
			response: "1. NO — false positive\n2. UNSURE — need more info\n3. YES — real\n",
			count:    3,
			want:     []VerificationStatus{StatusDismissed, StatusConfirmed, StatusConfirmed},
		},
		{
			name:     "case insensitive",
			response: "1. yes - confirmed\n2. No - not real\n",
			count:    2,
			want:     []VerificationStatus{StatusConfirmed, StatusDismissed},
		},
		{
			name:     "garbage response",
			response: "I think these findings are interesting...",
			count:    2,
			want:     []VerificationStatus{StatusConfirmed, StatusConfirmed}, // defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := parseHaikuResponse(tt.response, tt.count)
			if len(verdicts) != tt.count {
				t.Fatalf("expected %d verdicts, got %d", tt.count, len(verdicts))
			}
			for i, v := range verdicts {
				if v.Status != tt.want[i] {
					t.Errorf("verdict[%d] = %s, want %s", i, v.Status, tt.want[i])
				}
			}
		})
	}
}

func TestParseOpusResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     VerificationStatus
		wantRisk Risk
	}{
		{
			name:     "confirmed",
			response: `{"verdict": "confirmed", "reason": "real issue"}`,
			want:     StatusConfirmed,
		},
		{
			name:     "dismissed",
			response: `{"verdict": "dismissed", "reason": "nil check exists"}`,
			want:     StatusDismissed,
		},
		{
			name:     "downgraded",
			response: `{"verdict": "downgraded", "reason": "minor", "adjusted_risk": "info"}`,
			want:     StatusDowngraded,
			wantRisk: RiskInfo,
		},
		{
			name:     "json in code fence",
			response: "```json\n{\"verdict\": \"dismissed\", \"reason\": \"false positive\"}\n```",
			want:     StatusDismissed,
		},
		{
			name:     "garbage",
			response: "I can't parse this request properly",
			want:     StatusConfirmed, // safe fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := parseOpusResponse(tt.response)
			if v.Status != tt.want {
				t.Errorf("status = %s, want %s", v.Status, tt.want)
			}
			if tt.wantRisk != "" && v.AdjustedRisk != tt.wantRisk {
				t.Errorf("risk = %s, want %s", v.AdjustedRisk, tt.wantRisk)
			}
		})
	}
}

package agents

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
)

// TestIntegration_UnanimousConsensus verifies that when all agents find the
// same issue, dedup merges them into a single finding with VoteCount == TotalAgents.
func TestIntegration_UnanimousConsensus(t *testing.T) {
	t.Parallel()
	// Every agent returns the same critical finding with slightly different wording.
	responses := map[string]string{
		"sentinel":      `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.95,"summary":"SQL injection vulnerability","detail":"Query uses string concatenation."}]}`,
		"know-it-all":   `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.9,"summary":"SQL injection in database query","detail":"Raw SQL with user input."}]}`,
		"architect":     `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.92,"summary":"SQL injection attack vector","detail":"Parameterize the query."}]}`,
		"solver":        `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.88,"summary":"SQL injection in query construction","detail":"Use prepared statements."}]}`,
		"editor":        `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.85,"summary":"Unsafe SQL query with injection risk","detail":"Direct string interpolation."}]}`,
		"optimizer":     `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.8,"summary":"SQL injection risk in database query","detail":"Parameterize inputs."}]}`,
		"test-engineer": `{"findings":[{"file":"db.go","line":10,"risk":"critical","category":"security","scope":"changed","confidence":0.82,"summary":"SQL injection vulnerability in query","detail":"No parameterization."}]}`,
	}

	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			for slug, resp := range responses {
				if strings.Contains(req.SystemPrompt, slug) {
					return resp, llm.Usage{}, nil
				}
			}
			return `{"findings":[]}`, llm.Usage{}, nil
		},
	}

	orch := &Orchestrator{
		roles:  testRoles(),
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	result, err := orch.Review(realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	// All 7 agents found the same issue — should dedup to 1 finding.
	if len(result.DedupedFindings) != 1 {
		t.Errorf("expected 1 deduped finding (unanimous), got %d", len(result.DedupedFindings))
		for i, f := range result.DedupedFindings {
			t.Logf("  finding[%d]: %s (file=%s line=%d votes=%d)", i, f.Summary, f.File, f.Line, f.VoteCount)
		}
	}

	if len(result.DedupedFindings) > 0 {
		f := result.DedupedFindings[0]
		if f.VoteCount != f.TotalAgents {
			t.Errorf("unanimous finding should have VoteCount == TotalAgents, got %d/%d", f.VoteCount, f.TotalAgents)
		}
		// Score should be penalized — one unanimous critical deducts 20 points.
		if result.HealthScore.Score > 85 {
			t.Errorf("unanimous critical should penalize score, got %d", result.HealthScore.Score)
		}
	}
}

// TestIntegration_FindingsForFilesNotInPR verifies that agents can return
// findings for files not in the PR (e.g. codebase-scope observations).
// These should pass through and appear in the result.
func TestIntegration_FindingsForFilesNotInPR(t *testing.T) {
	t.Parallel()
	mock := &llmtest.Mock{
		Response: `{"findings":[
			{"file":"auth.go","line":10,"risk":"warning","category":"bug","scope":"changed","confidence":0.8,"summary":"Bug in PR file","detail":"d"},
			{"file":"unrelated.go","line":5,"risk":"info","category":"design","scope":"codebase","confidence":0.7,"summary":"Broader codebase observation","detail":"d"}
		]}`,
	}

	orch := &Orchestrator{
		roles:  testRoles()[:1],
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	pr := &gh.PR{
		Number: "1",
		Title:  "Test",
		Diff:   "diff",
		Files:  []gh.FileChange{{Path: "auth.go"}}, // Only auth.go is in the PR.
	}

	result, err := orch.Review(pr)
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	// Both findings should pass through — even the one for unrelated.go.
	if len(result.Findings) != 2 {
		t.Errorf("expected 2 findings (including out-of-PR file), got %d", len(result.Findings))
	}

	var hasCodebase bool
	for _, f := range result.Findings {
		if f.File == "unrelated.go" && f.Scope == ScopeCodebase {
			hasCodebase = true
		}
	}
	if !hasCodebase {
		t.Error("expected codebase-scope finding for unrelated.go to pass through")
	}
}

// TestIntegration_EmptyDiff verifies behavior when PR has an empty diff.
func TestIntegration_EmptyDiff(t *testing.T) {
	t.Parallel()
	mock := &llmtest.Mock{Response: `{"findings":[]}`}
	orch := &Orchestrator{
		roles:  testRoles()[:1],
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	pr := &gh.PR{Number: "1", Title: "Empty PR", Diff: "", Files: nil}
	result, err := orch.Review(pr)
	if err != nil {
		t.Fatalf("empty diff should not error: %v", err)
	}
	if result.HealthScore.Score != 100 {
		t.Errorf("empty diff with no findings should score 100, got %d", result.HealthScore.Score)
	}
}

// TestIntegration_AllAgentsTimeout verifies error when every agent fails.
func TestIntegration_AllAgentsTimeout(t *testing.T) {
	t.Parallel()
	mock := &llmtest.Mock{Err: context.DeadlineExceeded}
	orch := &Orchestrator{
		roles:  testRoles()[:2],
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	_, err := orch.Review(realisticPR())
	if err == nil {
		t.Fatal("expected error when all agents fail")
	}
	if !strings.Contains(err.Error(), "all agents failed") {
		t.Errorf("expected 'all agents failed' error, got: %v", err)
	}
}

// TestIntegration_MalformedLLMResponses verifies the pipeline handles
// various broken LLM outputs gracefully.
func TestIntegration_MalformedLLMResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "truncated JSON",
			response: `{"findings": [{"file": "a.go", "line": 10, "risk"`,
			wantErr:  true,
		},
		{
			name:     "wrong schema (errors instead of findings)",
			response: `{"errors": ["something went wrong"]}`,
			wantErr:  false, // Parses as empty findings — agent found nothing.
		},
		{
			name:     "findings as object instead of array",
			response: `{"findings": {"0": {"file": "a.go"}}}`,
			wantErr:  true,
		},
		{
			name:     "valid JSON but empty object",
			response: `{}`,
			wantErr:  false, // Parses as empty findings.
		},
		{
			name:     "prose with embedded JSON",
			response: "Here are my findings:\n```json\n" + `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"ok","detail":"d"}]}` + "\n```\nHope this helps!",
			wantErr:  false,
		},
		{
			name:     "JSON with extra fields (forward compat)",
			response: `{"findings":[{"file":"a.go","line":1,"risk":"info","summary":"ok","detail":"d","new_field":"value"}],"metadata":{}}`,
			wantErr:  false,
		},
		{
			name:     "completely empty response",
			response: "",
			wantErr:  true,
		},
		{
			name:     "null findings",
			response: `{"findings": null}`,
			wantErr:  false, // null parses as empty array.
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mock := &llmtest.Mock{Response: tt.response}
			orch := &Orchestrator{
				roles:  testRoles()[:1],
				opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
				skills: testSkills(),
				llm:    mock,
			}

			result, err := orch.Review(realisticPR())
			if tt.wantErr {
				// Malformed response should cause agent failure.
				// With 1 agent and 0 retries, this means all agents failed.
				if err == nil {
					t.Error("expected error for malformed response")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

// TestIntegration_SuggestionsOnlyForWarningPlusWithLine verifies that
// inline suggestions are only created for warning+ findings that have
// a file and line number.
func TestIntegration_SuggestionsOnlyForWarningPlusWithLine(t *testing.T) {
	t.Parallel()
	mock := &llmtest.Mock{
		Response: `{"findings":[
			{"file":"a.go","line":10,"risk":"critical","category":"bug","scope":"changed","confidence":0.9,"summary":"critical with line","detail":"d"},
			{"file":"a.go","line":20,"risk":"warning","category":"bug","scope":"changed","confidence":0.9,"summary":"warning with line","detail":"d"},
			{"file":"a.go","line":30,"risk":"info","category":"style","scope":"changed","confidence":0.9,"summary":"info with line","detail":"d"},
			{"file":"a.go","line":0,"risk":"critical","category":"bug","scope":"changed","confidence":0.9,"summary":"critical no line","detail":"d"},
			{"file":"","line":0,"risk":"critical","category":"bug","scope":"codebase","confidence":0.9,"summary":"critical no file","detail":"d"}
		]}`,
	}

	orch := &Orchestrator{
		roles:  testRoles()[:1],
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	result, err := orch.Review(realisticPR())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only findings with file + line > 0 + risk != info should become suggestions.
	// That's: critical@line10, warning@line20 = 2 suggestions.
	if len(result.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions (critical+warning with line), got %d", len(result.Suggestions))
		for _, s := range result.Suggestions {
			t.Logf("  suggestion: %s line=%d", s.File, s.Line)
		}
	}
}

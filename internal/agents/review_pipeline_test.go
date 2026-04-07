package agents

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/llm/llmtest"
)

// Realistic mock responses that mirror what Claude actually produces.
// Each role focuses on its specialty and may overlap on the same issues,
// which exercises the dedup and scoring pipeline.
var mockAgentResponses = map[string]string{
	"sentinel": `{"findings": [
		{
			"file": "auth/login.go",
			"line": 42,
			"risk": "critical",
			"category": "security",
			"scope": "changed",
			"confidence": 0.95,
			"summary": "SQL injection in login query",
			"detail": "User input is interpolated directly into the SQL query string without parameterization. An attacker could craft a username like ' OR 1=1 -- to bypass authentication.",
			"code_example": "// Before (vulnerable):\ndb.Query(\"SELECT * FROM users WHERE name = '\" + username + \"'\")\n\n// After (parameterized):\ndb.Query(\"SELECT * FROM users WHERE name = ?\", username)"
		},
		{
			"file": "auth/session.go",
			"line": 15,
			"risk": "warning",
			"category": "security",
			"scope": "changed",
			"confidence": 0.8,
			"summary": "Session token generated with weak randomness",
			"detail": "Using math/rand instead of crypto/rand for session token generation. math/rand is deterministic and predictable."
		}
	]}`,

	"know-it-all": `{"findings": [
		{
			"file": "auth/login.go",
			"line": 42,
			"risk": "critical",
			"category": "security",
			"scope": "changed",
			"confidence": 0.9,
			"summary": "Unsanitized SQL query allows injection",
			"detail": "The login function concatenates user input into a raw SQL string. Use parameterized queries instead."
		},
		{
			"file": "auth/login.go",
			"line": 58,
			"risk": "warning",
			"category": "design",
			"scope": "changed",
			"confidence": 0.75,
			"summary": "Error messages leak internal details",
			"detail": "Returning the raw database error to the client can expose schema details. Return a generic 'invalid credentials' message instead."
		},
		{
			"file": "auth/middleware.go",
			"line": 30,
			"risk": "info",
			"category": "style",
			"scope": "existing",
			"confidence": 0.6,
			"summary": "Consider extracting auth check into middleware",
			"detail": "The auth check is duplicated in three handlers. A middleware would reduce duplication."
		}
	]}`,

	"architect": `{"findings": [
		{
			"file": "auth/login.go",
			"line": 40,
			"risk": "critical",
			"category": "security",
			"scope": "changed",
			"confidence": 0.92,
			"summary": "SQL injection vulnerability in authentication",
			"detail": "String concatenation in SQL queries is a textbook injection vector. This is the authentication path, so exploitation grants full access."
		},
		{
			"file": "auth/session.go",
			"line": 10,
			"risk": "warning",
			"category": "design",
			"scope": "changed",
			"confidence": 0.85,
			"summary": "Session management should be a separate package",
			"detail": "Session creation, validation, and expiry are mixed into the auth package. Extract into internal/session for testability and reuse."
		}
	]}`,

	"solver": `{"findings": [
		{
			"file": "auth/login.go",
			"line": 42,
			"risk": "critical",
			"category": "bug",
			"scope": "changed",
			"confidence": 0.88,
			"summary": "Login query is vulnerable to SQL injection",
			"detail": "The WHERE clause uses string formatting. Use database/sql parameterized queries."
		},
		{
			"file": "auth/login.go",
			"line": 65,
			"risk": "info",
			"category": "design",
			"scope": "changed",
			"confidence": 0.55,
			"summary": "Missing rate limiting on login endpoint",
			"detail": "No rate limiting is applied to failed login attempts, enabling brute force attacks."
		}
	]}`,

	"editor": `{"findings": [
		{
			"file": "auth/login.go",
			"line": 20,
			"risk": "info",
			"category": "style",
			"scope": "changed",
			"confidence": 0.7,
			"summary": "Function is too long (85 lines)",
			"detail": "handleLogin does validation, query, session creation, and response formatting. Consider splitting into smaller functions."
		},
		{
			"file": "auth/middleware.go",
			"line": 1,
			"risk": "info",
			"category": "style",
			"scope": "existing",
			"confidence": 0.5,
			"summary": "Package comment missing",
			"detail": "The auth package has no package-level doc comment."
		}
	]}`,

	"optimizer": `{"findings": [
		{
			"file": "auth/session.go",
			"line": 45,
			"risk": "warning",
			"category": "performance",
			"scope": "changed",
			"confidence": 0.7,
			"summary": "Session lookup scans all sessions",
			"detail": "ValidateSession iterates over all active sessions in a slice. Use a map keyed by token for O(1) lookup."
		}
	]}`,

	"test-engineer": `{"findings": [
		{
			"file": "auth/login_test.go",
			"line": 0,
			"risk": "warning",
			"category": "testing",
			"scope": "changed",
			"confidence": 0.8,
			"summary": "No test for SQL injection edge cases",
			"detail": "The test suite covers happy-path login but does not test malicious input like SQL injection payloads or unicode edge cases."
		},
		{
			"file": "auth/session_test.go",
			"line": 0,
			"risk": "info",
			"category": "testing",
			"scope": "codebase",
			"confidence": 0.6,
			"summary": "Session expiry not tested",
			"detail": "No tests verify that expired sessions are correctly rejected."
		}
	]}`,
}

// realisticPR returns a PR that looks like a real auth feature addition.
func realisticPR() *gh.PR {
	return &gh.PR{
		Number: "42",
		Title:  "Add user authentication with session management",
		Body:   "Implements login endpoint with session-based auth. Uses database/sql for user lookup.",
		Repo:   "arinorr/myapp",
		Diff: `diff --git a/auth/login.go b/auth/login.go
new file mode 100644
--- /dev/null
+++ b/auth/login.go
@@ -0,0 +1,85 @@
+package auth
+
+import (
+	"database/sql"
+	"net/http"
+)
+
+func handleLogin(db *sql.DB) http.HandlerFunc {
+	return func(w http.ResponseWriter, r *http.Request) {
+		username := r.FormValue("username")
+		password := r.FormValue("password")
+
+		query := "SELECT id, password_hash FROM users WHERE name = '" + username + "'"
+		row := db.QueryRow(query)
+		// ... rest of handler
+	}
+}
diff --git a/auth/session.go b/auth/session.go
new file mode 100644
--- /dev/null
+++ b/auth/session.go
@@ -0,0 +1,50 @@
+package auth
+
+import "math/rand"
+
+func generateToken() string {
+	b := make([]byte, 32)
+	for i := range b {
+		b[i] = byte(rand.Intn(256))
+	}
+	return string(b)
+}
diff --git a/auth/middleware.go b/auth/middleware.go
--- a/auth/middleware.go
+++ b/auth/middleware.go
@@ -28,6 +28,10 @@
+func requireAuth(next http.Handler) http.Handler {
+	// ... existing middleware
+}`,
		Files: []gh.FileChange{
			{Path: "auth/login.go", Status: "added"},
			{Path: "auth/session.go", Status: "added"},
			{Path: "auth/middleware.go", Status: "modified"},
			{Path: "auth/login_test.go", Status: "added"},
			{Path: "auth/session_test.go", Status: "added"},
		},
	}
}

// perRoleMock returns a mock LLM that returns role-specific responses.
func perRoleMock() *llmtest.Mock {
	return &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			// The system prompt contains the skill file content. Match the
			// role by checking which slug's response map entry to return.
			// In practice the orchestrator sets the system prompt to the
			// skill content, so we look at the user prompt which contains
			// the PR diff. But we need to identify the role — we do this
			// by inspecting the system prompt for role-specific keywords,
			// or just rotate through responses.
			for slug, response := range mockAgentResponses {
				if strings.Contains(req.SystemPrompt, slug) {
					return response, llm.Usage{
						InputTokens:  5000,
						OutputTokens: 800,
						CostUSD:      0.05,
						DurationMS:   2000,
					}, nil
				}
			}
			// Fallback: return empty findings.
			return `{"findings": []}`, llm.Usage{InputTokens: 1000, OutputTokens: 100, CostUSD: 0.01}, nil
		},
	}
}

// testRoles returns roles with inline skill content that includes the slug
// so perRoleMock can identify which role is being called.
func testRoles() []Role {
	return []Role{
		{Name: "Sentinel", Slug: "sentinel", SkillFile: "sentinel.md", Model: "opus"},
		{Name: "Know-It-All", Slug: "know-it-all", SkillFile: "know-it-all.md", Model: "sonnet"},
		{Name: "Architect", Slug: "architect", SkillFile: "architect.md", Model: "opus"},
		{Name: "Solver", Slug: "solver", SkillFile: "solver.md", Model: "sonnet"},
		{Name: "Editor", Slug: "editor", SkillFile: "editor.md", Model: "haiku"},
		{Name: "Optimizer", Slug: "optimizer", SkillFile: "optimizer.md", Model: "sonnet"},
		{Name: "Test Engineer", Slug: "test-engineer", SkillFile: "test-engineer.md", Model: "haiku"},
	}
}

// testSkills returns a skill map where each skill contains the role slug,
// so the mock can identify which role made the request.
func testSkills() map[string]string {
	skills := make(map[string]string)
	for _, r := range testRoles() {
		skills[r.Slug] = "You are the " + r.Slug + " reviewer."
	}
	return skills
}

// TestIntegration_FullReviewPipeline exercises the complete path:
// dispatch agents → parse responses → deduplicate → score → summarize.
func TestIntegration_FullReviewPipeline(t *testing.T) {
	t.Parallel()
	roles := testRoles()
	orch := &Orchestrator{
		roles:  roles,
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    perRoleMock(),
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	// All 7 agents should succeed.
	if len(result.FailedAgents) != 0 {
		t.Errorf("expected no failed agents, got %v", result.FailedAgents)
	}

	// We expect findings from all agents.
	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	// The SQL injection finding appears in 4 agents (sentinel, know-it-all,
	// architect, solver) with similar descriptions. Dedup should merge them.
	if len(result.DedupedFindings) == 0 {
		t.Fatal("expected deduped findings, got none")
	}
	if len(result.DedupedFindings) >= len(result.Findings) {
		t.Errorf("dedup should reduce finding count: %d deduped >= %d raw",
			len(result.DedupedFindings), len(result.Findings))
	}

	// The SQL injection finding should have high consensus (3+ votes).
	var sqlInjection *DedupedFinding
	for i := range result.DedupedFindings {
		f := &result.DedupedFindings[i]
		if f.File == "auth/login.go" && f.Risk == RiskCritical {
			sqlInjection = f
			break
		}
	}
	if sqlInjection == nil {
		t.Fatal("expected a critical finding for auth/login.go")
	}
	if sqlInjection.VoteCount < 3 {
		t.Errorf("SQL injection should have 3+ votes (found by sentinel, know-it-all, architect, solver), got %d", sqlInjection.VoteCount)
	}

	// Health score should be computed and reflect the critical finding.
	if result.HealthScore.Score == 0 {
		t.Error("expected non-zero health score")
	}
	if result.HealthScore.Score >= 100 {
		t.Errorf("score should be penalized by findings, got %d", result.HealthScore.Score)
	}
	if result.HealthScore.Grade == "" {
		t.Error("expected a grade")
	}
	if result.HealthScore.Verdict == "" {
		t.Error("expected a verdict")
	}

	// Summary should be a non-empty deterministic string.
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if !strings.Contains(result.Summary, "critical") {
		t.Error("summary should mention critical findings")
	}

	// Suggestions should be generated for warning+ findings with file+line.
	if len(result.Suggestions) == 0 {
		t.Error("expected inline suggestions for warning+ findings")
	}

	// Token usage should be aggregated across all agents.
	if result.Usage.InputTokens == 0 {
		t.Error("expected non-zero aggregated input tokens")
	}
	if result.Usage.CostUSD == 0 {
		t.Error("expected non-zero aggregated cost")
	}

	// Per-agent usage should be tracked.
	if len(result.AgentUsages) != len(roles) {
		t.Errorf("expected %d agent usages, got %d", len(roles), len(result.AgentUsages))
	}
}

// TestIntegration_ScopeDistribution verifies findings are correctly bucketed
// by scope (changed, existing, codebase).
func TestIntegration_ScopeDistribution(t *testing.T) {
	t.Parallel()
	orch := &Orchestrator{
		roles:  testRoles(),
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    perRoleMock(),
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	var changed, existing, codebase int
	for _, f := range result.DedupedFindings {
		switch f.Scope {
		case ScopeChanged:
			changed++
		case ScopeExisting:
			existing++
		case ScopeCodebase:
			codebase++
		}
	}

	// Our mock data has findings in all three scopes.
	if changed == 0 {
		t.Error("expected findings in 'changed' scope")
	}
	// Existing and codebase may or may not survive dedup, so we check raw.
	var rawExisting, rawCodebase int
	for _, f := range result.Findings {
		switch f.Scope {
		case ScopeExisting:
			rawExisting++
		case ScopeCodebase:
			rawCodebase++
		}
	}
	if rawExisting == 0 {
		t.Error("expected raw findings in 'existing' scope")
	}
	if rawCodebase == 0 {
		t.Error("expected raw findings in 'codebase' scope")
	}
}

// TestIntegration_PartialFailureProducesResults verifies that the pipeline
// still produces a valid result when some agents fail.
func TestIntegration_PartialFailureProducesResults(t *testing.T) {
	t.Parallel()
	var callCount int32
	mock := &llmtest.Mock{
		CompleteFunc: func(_ context.Context, req llm.Request) (string, llm.Usage, error) {
			// Fail every other agent using atomic counter for thread safety.
			n := atomic.AddInt32(&callCount, 1)
			if n%2 == 0 {
				return "", llm.Usage{}, context.DeadlineExceeded
			}
			return `{"findings": [{"file": "a.go", "line": 1, "risk": "warning", "category": "bug", "scope": "changed", "confidence": 0.8, "summary": "issue", "detail": "details"}]}`,
				llm.Usage{InputTokens: 1000, OutputTokens: 200, CostUSD: 0.02}, nil
		},
	}

	orch := &Orchestrator{
		roles:  testRoles(),
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("partial failure should not error: %v", err)
	}

	if len(result.FailedAgents) == 0 {
		t.Error("expected some failed agents")
	}
	if len(result.Findings) == 0 {
		t.Error("expected findings from successful agents")
	}
	if result.HealthScore.Score == 0 {
		t.Error("expected a computed health score despite partial failure")
	}
}

// TestIntegration_CleanPR verifies that agents returning empty findings
// produce a healthy score.
func TestIntegration_CleanPR(t *testing.T) {
	t.Parallel()
	orch := &Orchestrator{
		roles:  testRoles(),
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    &llmtest.Mock{Response: `{"findings": []}`},
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings for clean PR, got %d", len(result.Findings))
	}
	if result.HealthScore.Score != 100 {
		t.Errorf("expected score 100 for clean PR, got %d", result.HealthScore.Score)
	}
	if result.HealthScore.Grade != "A+" {
		t.Errorf("expected grade A+ for clean PR, got %q", result.HealthScore.Grade)
	}
}

// TestIntegration_LowConfidenceFindingsFiltered verifies that findings below
// the confidence threshold are dropped during parsing.
func TestIntegration_LowConfidenceFindingsFiltered(t *testing.T) {
	t.Parallel()
	mock := &llmtest.Mock{
		Response: `{"findings": [
			{"file": "a.go", "line": 1, "risk": "warning", "category": "bug", "scope": "changed", "confidence": 0.9, "summary": "high confidence", "detail": "d"},
			{"file": "a.go", "line": 2, "risk": "warning", "category": "bug", "scope": "changed", "confidence": 0.2, "summary": "low confidence", "detail": "d"},
			{"file": "a.go", "line": 3, "risk": "info", "category": "style", "scope": "changed", "confidence": 0.1, "summary": "very low confidence", "detail": "d"}
		]}`,
	}

	orch := &Orchestrator{
		roles:  testRoles()[:1], // Just one agent to keep it simple.
		opts:   &Options{Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    mock,
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	// Only the high-confidence finding should survive (confidence >= 0.5).
	for _, f := range result.Findings {
		if f.Confidence < ConfidenceThreshold {
			t.Errorf("finding with confidence %.2f should have been filtered: %q", f.Confidence, f.Summary)
		}
	}
}

// TestIntegration_VerboseTracksPerAgentUsage verifies that per-agent usage
// is collected even with verbose logging enabled.
func TestIntegration_VerboseTracksPerAgentUsage(t *testing.T) {
	t.Parallel()
	roles := testRoles()[:3] // Sentinel, Know-It-All, Architect
	orch := &Orchestrator{
		roles:  roles,
		opts:   &Options{Verbose: true, Out: io.Discard, ErrOut: io.Discard},
		skills: testSkills(),
		llm:    perRoleMock(),
	}

	result, err := orch.Review(context.Background(), realisticPR())
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	if len(result.AgentUsages) != len(roles) {
		t.Errorf("expected %d agent usages, got %d", len(roles), len(result.AgentUsages))
	}

	// Each agent should report non-zero usage.
	for _, au := range result.AgentUsages {
		if au.Usage.InputTokens == 0 {
			t.Errorf("agent %s should have non-zero input tokens", au.Role)
		}
		if au.Role == "" {
			t.Error("agent usage should have a role name")
		}
	}

	// Aggregated usage should equal sum of per-agent usage.
	var totalCost float64
	for _, au := range result.AgentUsages {
		totalCost += au.Usage.CostUSD
	}
	if result.Usage.CostUSD != totalCost {
		t.Errorf("aggregated cost $%.2f != sum of per-agent cost $%.2f", result.Usage.CostUSD, totalCost)
	}
}

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

	result, err := orch.Review(context.Background(), realisticPR())
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
		// Score should be penalized — one unanimous critical deducts ~9 points.
		if result.HealthScore.Score > 95 {
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

	result, err := orch.Review(context.Background(), pr)
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
	result, err := orch.Review(context.Background(), pr)
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

	_, err := orch.Review(context.Background(), realisticPR())
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

			result, err := orch.Review(context.Background(), realisticPR())
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

	result, err := orch.Review(context.Background(), realisticPR())
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

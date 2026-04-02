package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestParseFeedback_DirectJSON(t *testing.T) {
	input := `{"findings": [{"file": "main.go", "line": 10, "severity": "warning", "summary": "test", "detail": "detail"}]}`
	fb, err := parseFeedback("test-role", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb.Role != "test-role" {
		t.Errorf("expected role 'test-role', got %q", fb.Role)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	f := fb.Findings[0]
	if f.File != "main.go" || f.Line != 10 || f.Severity != "warning" {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestParseFeedback_MarkdownCodeBlock(t *testing.T) {
	input := "Here are my findings:\n```json\n" +
		`{"findings": [{"file": "foo.go", "line": 1, "severity": "info", "summary": "s", "detail": "d"}]}` +
		"\n```\nHope this helps!"
	fb, err := parseFeedback("editor", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].File != "foo.go" {
		t.Errorf("expected file 'foo.go', got %q", fb.Findings[0].File)
	}
}

func TestParseFeedback_ProseWrappedJSON(t *testing.T) {
	input := `Now I have analyzed the code thoroughly.

{"findings": [{"file": "cmd/root.go", "line": 5, "severity": "critical", "summary": "bug", "detail": "details here"}]}

That concludes my review.`
	fb, err := parseFeedback("solver", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].Severity != "critical" {
		t.Errorf("expected severity 'critical', got %q", fb.Findings[0].Severity)
	}
}

func TestParseFeedback_EmptyFindings(t *testing.T) {
	input := `{"findings": []}`
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(fb.Findings))
	}
}

func TestParseFeedback_InvalidJSON(t *testing.T) {
	input := "This is not JSON at all, just plain text without any braces."
	_, err := parseFeedback("test", input)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "could not extract JSON") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseFeedback_MultipleCodeBlocks(t *testing.T) {
	input := "First block is not JSON:\n```\nsome text\n```\n\nSecond block has it:\n```json\n" +
		`{"findings": [{"file": "a.go", "line": 1, "severity": "info", "summary": "s", "detail": "d"}]}` +
		"\n```"
	fb, err := parseFeedback("test", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
}

func TestTruncateUTF8_Short(t *testing.T) {
	s := "hello"
	got := truncateUTF8(s, 10)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_ExactLength(t *testing.T) {
	s := "hello"
	got := truncateUTF8(s, 5)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_Truncates(t *testing.T) {
	s := "hello world"
	got := truncateUTF8(s, 5)
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestTruncateUTF8_MultiByte(t *testing.T) {
	// "é" is 2 bytes in UTF-8. Cutting at byte 1 should back up.
	s := "é"
	got := truncateUTF8(s, 1)
	if got != "" {
		t.Errorf("expected empty string (can't fit the rune), got %q", got)
	}
}

func TestTruncateUTF8_MultiBytePreserved(t *testing.T) {
	// "aé" = 'a' (1 byte) + 'é' (2 bytes) = 3 bytes total.
	s := "aé"
	got := truncateUTF8(s, 3)
	if got != "aé" {
		t.Errorf("expected 'aé', got %q", got)
	}
	// Truncate at 2 bytes should keep just 'a'.
	got = truncateUTF8(s, 2)
	if got != "a" {
		t.Errorf("expected 'a', got %q", got)
	}
}

func TestBuildAgentPrompt(t *testing.T) {
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{
		Title: "Fix bug",
		Body:  "This fixes the bug",
		Diff:  "+ added line",
	}
	prompt := buildAgentPrompt(role, pr)
	if !strings.Contains(prompt, "Fix bug") {
		t.Error("prompt should contain PR title")
	}
	if !strings.Contains(prompt, "This fixes the bug") {
		t.Error("prompt should contain PR body")
	}
	if !strings.Contains(prompt, "+ added line") {
		t.Error("prompt should contain diff")
	}
	if !strings.Contains(prompt, `{"findings"`) {
		t.Error("prompt should contain output format instructions")
	}
}

func TestSuggestionsFilterInfoSeverity(t *testing.T) {
	// Simulate what synthesize does: only warning+ findings become suggestions.
	feedbacks := []Feedback{
		{Role: "editor", Findings: []Finding{
			{File: "a.go", Line: 10, Severity: "info", Summary: "minor note", Detail: "detail"},
			{File: "a.go", Line: 20, Severity: "warning", Summary: "should fix", Detail: "detail"},
			{File: "a.go", Line: 30, Severity: "critical", Summary: "must fix", Detail: "detail"},
		}},
	}

	var suggestions []gh.Suggestion
	for _, fb := range feedbacks {
		for _, f := range fb.Findings {
			if f.File != "" && f.Line > 0 && (f.Severity == "warning" || f.Severity == "critical") {
				suggestions = append(suggestions, gh.Suggestion{
					File: f.File,
					Line: f.Line,
					Role: fb.Role,
				})
			}
		}
	}

	if len(suggestions) != 2 {
		t.Fatalf("expected 2 suggestions (warning + critical), got %d", len(suggestions))
	}
	if suggestions[0].Line != 20 {
		t.Errorf("first suggestion should be line 20 (warning), got %d", suggestions[0].Line)
	}
	if suggestions[1].Line != 30 {
		t.Errorf("second suggestion should be line 30 (critical), got %d", suggestions[1].Line)
	}
}

func TestFindingRoleStamped(t *testing.T) {
	fb := Feedback{
		Role: "sentinel",
		Findings: []Finding{
			{File: "a.go", Severity: "warning", Summary: "test"},
		},
	}
	// Simulate the role-stamping logic from synthesize.
	f := fb.Findings[0]
	f.Role = fb.Role
	if f.Role != "sentinel" {
		t.Errorf("expected role 'sentinel', got %q", f.Role)
	}
}

func TestBuildSynthesisPrompt(t *testing.T) {
	pr := &gh.PR{Title: "Test PR"}
	feedbacks := []Feedback{
		{Role: "know-it-all", Findings: []Finding{{File: "a.go", Severity: "info", Summary: "test"}}},
		{Role: "editor", Findings: []Finding{{File: "b.go", Severity: "warning", Summary: "test2"}}},
	}
	prompt := buildSynthesisPrompt(pr, feedbacks)
	if !strings.Contains(prompt, "Test PR") {
		t.Error("synthesis prompt should contain PR title")
	}
	if !strings.Contains(prompt, "know-it-all") {
		t.Error("synthesis prompt should contain agent feedback")
	}
	if !strings.Contains(prompt, "editor") {
		t.Error("synthesis prompt should contain all agents' feedback")
	}
}

func TestNewOrchestrator_LoadsSkills(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "test-skill.md")
	if err := os.WriteFile(skillPath, []byte("You are a test reviewer."), 0o644); err != nil {
		t.Fatal(err)
	}

	roles := []Role{{Name: "Test", Slug: "test", SkillFile: skillPath}}
	orch, err := NewOrchestrator(roles, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if orch.skill(roles[0]) != "You are a test reviewer." {
		t.Errorf("skill content mismatch: %q", orch.skill(roles[0]))
	}
}

func TestNewOrchestrator_MissingSkillFile(t *testing.T) {
	roles := []Role{{Name: "Bad", Slug: "bad", SkillFile: "/nonexistent/path.md"}}
	_, err := NewOrchestrator(roles, Options{})
	if err == nil {
		t.Fatal("expected error for missing skill file")
	}
	if !strings.Contains(err.Error(), "failed to load skill") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewOrchestrator_EmptyRoles(t *testing.T) {
	orch, err := NewOrchestrator([]Role{}, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orch.roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(orch.roles))
	}
}

func TestReadSkillFile_DirectPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := readSkillFile(path, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("expected 'content', got %q", string(data))
	}
}

func TestReadSkillFile_FallbackToExeDir(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "test.md"), []byte("fallback"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Direct path won't work, but exeDir fallback should.
	data, err := readSkillFile("skills/test.md", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "fallback" {
		t.Errorf("expected 'fallback', got %q", string(data))
	}
}

func TestReadSkillFile_NotFound(t *testing.T) {
	_, err := readSkillFile("/nonexistent.md", "/also/nonexistent")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDryRun_EmptyRoles(t *testing.T) {
	orch := &Orchestrator{roles: []Role{}, opts: Options{DryRun: true}, skills: map[string]string{}}
	pr := &gh.PR{Number: "1", Title: "Test"}
	_, err := orch.Review(pr)
	if err == nil {
		t.Fatal("expected error for empty roles in dry run")
	}
	if !strings.Contains(err.Error(), "no roles selected") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDryRun_ProducesResult(t *testing.T) {
	orch := &Orchestrator{
		roles:  []Role{{Name: "Test", Slug: "test", Description: "A test role"}},
		opts:   Options{DryRun: true},
		skills: map[string]string{"test": "skill content"},
	}
	pr := &gh.PR{Number: "42", Title: "Test PR", Diff: "some diff", Files: []gh.FileChange{{Path: "a.go"}}}
	result, err := orch.Review(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Summary, "dry run") {
		t.Errorf("dry run summary should mention dry run: %q", result.Summary)
	}
}

func TestDryRun_Verbose(t *testing.T) {
	orch := &Orchestrator{
		roles:  []Role{{Name: "Test", Slug: "test", Description: "A test role", SkillFile: "test.md"}},
		opts:   Options{DryRun: true, Verbose: true},
		skills: map[string]string{"test": "skill content here"},
	}
	pr := &gh.PR{Number: "1", Title: "Test", Diff: "diff"}
	result, err := orch.Review(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSkill_ReturnsContent(t *testing.T) {
	orch := &Orchestrator{
		skills: map[string]string{"sentinel": "security reviewer"},
	}
	role := Role{Slug: "sentinel"}
	if got := orch.skill(role); got != "security reviewer" {
		t.Errorf("expected 'security reviewer', got %q", got)
	}
}

func TestSkill_MissingReturnsEmpty(t *testing.T) {
	orch := &Orchestrator{skills: map[string]string{}}
	role := Role{Slug: "nonexistent"}
	if got := orch.skill(role); got != "" {
		t.Errorf("expected empty string for missing skill, got %q", got)
	}
}

func mockRunner(response string) claudeRunner {
	return func(args ...string) ([]byte, error) {
		return []byte(response), nil
	}
}

func mockRunnerJSON(findingsJSON string) claudeRunner {
	// The claude --output-format json wraps the response in {"result": "..."}
	// where the inner value is the raw text the model produced.
	inner := `{"findings":[` + findingsJSON + `]}`
	// Build the outer JSON properly.
	outerBytes, err := json.Marshal(struct {
		Result string `json:"result"`
	}{Result: inner})
	if err != nil {
		panic(err)
	}
	return func(args ...string) ([]byte, error) {
		return outerBytes, nil
	}
}

func TestRunAgent_Success(t *testing.T) {
	finding := `{"file":"a.go","line":1,"severity":"info","summary":"test","detail":"d"}`
	orch := &Orchestrator{
		skills: map[string]string{"test": "skill"},
		opts:   Options{},
		run:    mockRunnerJSON(finding),
	}
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{Title: "Test", Body: "body", Diff: "diff"}
	fb, err := orch.runAgent(role, pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fb.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(fb.Findings))
	}
	if fb.Findings[0].File != "a.go" {
		t.Errorf("expected file 'a.go', got %q", fb.Findings[0].File)
	}
}

func TestRunAgent_Verbose(t *testing.T) {
	finding := `{"file":"a.go","line":1,"severity":"info","summary":"s","detail":"d"}`
	orch := &Orchestrator{
		skills: map[string]string{"test": "skill"},
		opts:   Options{Verbose: true},
		run:    mockRunnerJSON(finding),
	}
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{Title: "Test", Body: "b", Diff: "d"}
	_, err := orch.runAgent(role, pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAgent_CommandFailure(t *testing.T) {
	orch := &Orchestrator{
		skills: map[string]string{"test": "skill"},
		opts:   Options{},
		run: func(args ...string) ([]byte, error) {
			return nil, fmt.Errorf("command failed")
		},
	}
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{Title: "Test", Body: "b", Diff: "d"}
	_, err := orch.runAgent(role, pr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "claude command failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunAgent_InvalidJSON(t *testing.T) {
	orch := &Orchestrator{
		skills: map[string]string{"test": "skill"},
		opts:   Options{},
		run:    mockRunner("not json at all"),
	}
	role := Role{Name: "Test", Slug: "test"}
	pr := &gh.PR{Title: "Test", Body: "b", Diff: "d"}
	_, err := orch.runAgent(role, pr)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDispatchAgents_AllSucceed(t *testing.T) {
	finding := `{"file":"a.go","line":1,"severity":"info","summary":"s","detail":"d"}`
	orch := &Orchestrator{
		roles:  []Role{{Name: "A", Slug: "a"}, {Name: "B", Slug: "b"}},
		skills: map[string]string{"a": "skill a", "b": "skill b"},
		opts:   Options{},
		run:    mockRunnerJSON(finding),
	}
	pr := &gh.PR{Title: "Test", Body: "b", Diff: "d"}
	feedbacks, err := orch.dispatchAgents(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feedbacks) != 2 {
		t.Errorf("expected 2 feedbacks, got %d", len(feedbacks))
	}
}

func TestDispatchAgents_AllFail(t *testing.T) {
	orch := &Orchestrator{
		roles:  []Role{{Name: "A", Slug: "a"}},
		skills: map[string]string{"a": "skill"},
		opts:   Options{},
		run: func(args ...string) ([]byte, error) {
			return nil, fmt.Errorf("fail")
		},
	}
	pr := &gh.PR{Title: "Test", Body: "b", Diff: "d"}
	_, err := orch.dispatchAgents(pr)
	if err == nil {
		t.Fatal("expected error when all agents fail")
	}
	if !strings.Contains(err.Error(), "all agents failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSynthesize_Success(t *testing.T) {
	orch := &Orchestrator{
		opts: Options{},
		run:  mockRunner("Overall the code looks good."),
	}
	pr := &gh.PR{Title: "Test"}
	feedbacks := []Feedback{
		{Role: "test", Findings: []Finding{
			{File: "a.go", Line: 10, Severity: "warning", Summary: "issue", Detail: "detail"},
		}},
	}
	result, err := orch.synthesize(pr, feedbacks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Summary, "looks good") {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
	if len(result.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion (warning with file+line), got %d", len(result.Suggestions))
	}
}

func TestSynthesize_InfoNotInSuggestions(t *testing.T) {
	orch := &Orchestrator{
		opts: Options{},
		run:  mockRunner("summary"),
	}
	pr := &gh.PR{Title: "Test"}
	feedbacks := []Feedback{
		{Role: "test", Findings: []Finding{
			{File: "a.go", Line: 5, Severity: "info", Summary: "note", Detail: "d"},
		}},
	}
	result, err := orch.synthesize(pr, feedbacks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Suggestions) != 0 {
		t.Errorf("info findings should not become suggestions, got %d", len(result.Suggestions))
	}
}

func TestSynthesize_CommandFailure(t *testing.T) {
	orch := &Orchestrator{
		opts: Options{},
		run: func(args ...string) ([]byte, error) {
			return nil, fmt.Errorf("synthesis error")
		},
	}
	pr := &gh.PR{Title: "Test"}
	_, err := orch.synthesize(pr, []Feedback{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "synthesis failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReview_FullPipeline(t *testing.T) {
	innerFindings := `{"findings":[{"file":"a.go","line":1,"severity":"warning","summary":"s","detail":"d"}]}`
	agentResponse, marshalErr := json.Marshal(struct {
		Result string `json:"result"`
	}{Result: innerFindings})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}

	callCount := 0
	orch := &Orchestrator{
		roles:  []Role{{Name: "Test", Slug: "test"}},
		skills: map[string]string{"test": "skill"},
		opts:   Options{},
		run: func(args ...string) ([]byte, error) {
			callCount++
			if callCount <= 1 {
				return agentResponse, nil
			}
			return []byte("Review complete."), nil
		},
	}
	pr := &gh.PR{Number: "1", Title: "Test", Body: "b", Diff: "d"}
	result, err := orch.Review(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Summary, "Review complete") {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
}

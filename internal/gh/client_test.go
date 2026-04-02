package gh

import (
	"fmt"
	"strings"
	"testing"
)

func TestGitStatusToString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"A", "added"},
		{"M", "modified"},
		{"D", "removed"},
		{"R", "renamed"},
		{"X", "modified"},
		{"", "modified"},
	}
	for _, tt := range tests {
		got := gitStatusToString(tt.input)
		if got != tt.want {
			t.Errorf("gitStatusToString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDetectBaseBranch(t *testing.T) {
	client, _ := NewClient()
	branch := client.detectBaseBranch()
	if branch != "main" && branch != "master" {
		t.Errorf("expected 'main' or 'master', got %q", branch)
	}
}

func TestDetectBaseBranch_FallsBackToMain(t *testing.T) {
	client := &Client{
		run: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	branch := client.detectBaseBranch()
	if branch != "main" {
		t.Errorf("expected fallback 'main', got %q", branch)
	}
}

func TestDetectBaseBranch_FindsMaster(t *testing.T) {
	client := &Client{
		run: func(name string, args ...string) ([]byte, error) {
			// Fail for "main", succeed for "master".
			for _, a := range args {
				if a == "main" {
					return nil, fmt.Errorf("not found")
				}
			}
			return []byte("ok"), nil
		},
	}
	branch := client.detectBaseBranch()
	if branch != "master" {
		t.Errorf("expected 'master', got %q", branch)
	}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.run == nil {
		t.Error("run should be set")
	}
	if client.exec == nil {
		t.Error("exec should be set")
	}
}

func TestGetPRDiff_RoutesToGH(t *testing.T) {
	callCount := 0
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			callCount++
			if name != "gh" {
				t.Errorf("expected gh command, got %q", name)
			}
			switch callCount {
			case 1: // gh pr view --json metadata
				return []byte(`{"number":42,"title":"Test PR","body":"desc","headRefOid":"abc123"}`), nil
			case 2: // gh pr diff
				return []byte("+ added line\n"), nil
			case 3: // gh pr view --json files
				return []byte(`{"files":[{"path":"main.go","additions":1,"deletions":0}]}`), nil
			}
			return nil, fmt.Errorf("unexpected call %d", callCount)
		},
	}

	pr, err := client.GetPRDiff("42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != "42" {
		t.Errorf("expected number '42', got %q", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("expected title 'Test PR', got %q", pr.Title)
	}
	if pr.HeadSHA != "abc123" {
		t.Errorf("expected HeadSHA 'abc123', got %q", pr.HeadSHA)
	}
	if len(pr.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(pr.Files))
	}
	if pr.Files[0].Path != "main.go" {
		t.Errorf("expected file 'main.go', got %q", pr.Files[0].Path)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestGetPRDiffGH_MetadataError(t *testing.T) {
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	_, err := client.GetPRDiff("999")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gh pr view failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGH_InvalidMetadataJSON(t *testing.T) {
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
	}
	_, err := client.GetPRDiff("1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to parse PR metadata") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGH_DiffError(t *testing.T) {
	callCount := 0
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			callCount++
			if callCount == 1 {
				return []byte(`{"number":1,"title":"t","body":"b","headRefOid":"sha"}`), nil
			}
			return nil, fmt.Errorf("diff error")
		},
	}
	_, err := client.GetPRDiff("1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gh pr diff failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGH_FilesError(t *testing.T) {
	callCount := 0
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				return []byte(`{"number":1,"title":"t","body":"b","headRefOid":"sha"}`), nil
			case 2:
				return []byte("diff"), nil
			}
			return nil, fmt.Errorf("files error")
		},
	}
	_, err := client.GetPRDiff("1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "gh pr view files failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGH_InvalidFilesJSON(t *testing.T) {
	callCount := 0
	client := &Client{
		useGH: true,
		run: func(name string, args ...string) ([]byte, error) {
			callCount++
			switch callCount {
			case 1:
				return []byte(`{"number":1,"title":"t","body":"b","headRefOid":"sha"}`), nil
			case 2:
				return []byte("diff"), nil
			case 3:
				return []byte("bad json"), nil
			}
			return nil, fmt.Errorf("unexpected")
		},
	}
	_, err := client.GetPRDiff("1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to parse files") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGit(t *testing.T) {
	client := &Client{
		useGH: false,
		run: func(name string, args ...string) ([]byte, error) {
			if name != "git" {
				t.Errorf("expected git command, got %q", name)
			}
			// Route by the git subcommand.
			if len(args) == 0 {
				return nil, fmt.Errorf("no args")
			}
			switch args[0] {
			case "rev-parse":
				if len(args) >= 3 && args[1] == "--verify" {
					// detectBaseBranch check
					return []byte("ok"), nil
				}
				// rev-parse HEAD
				return []byte("deadbeef\n"), nil
			case "diff":
				if len(args) >= 2 && args[1] == "--name-status" {
					return []byte("M\tmain.go\nA\tnew.go\n"), nil
				}
				return []byte("+ added\n- removed\n"), nil
			}
			return nil, fmt.Errorf("unexpected git subcommand: %s", args[0])
		},
	}

	pr, err := client.GetPRDiff("test-ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != "test-ref" {
		t.Errorf("expected number 'test-ref', got %q", pr.Number)
	}
	if pr.HeadSHA != "deadbeef" {
		t.Errorf("expected HeadSHA 'deadbeef', got %q", pr.HeadSHA)
	}
	if len(pr.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(pr.Files))
	}
	if pr.Files[0].Status != "modified" {
		t.Errorf("expected 'modified', got %q", pr.Files[0].Status)
	}
	if pr.Files[1].Status != "added" {
		t.Errorf("expected 'added', got %q", pr.Files[1].Status)
	}
}

func TestGetPRDiffGit_DiffError(t *testing.T) {
	client := &Client{
		useGH: false,
		run: func(name string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
				return []byte("ok"), nil // detectBaseBranch
			}
			return nil, fmt.Errorf("git error")
		},
	}
	_, err := client.GetPRDiff("ref")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git diff failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPRDiffGit_NameStatusError(t *testing.T) {
	client := &Client{
		useGH: false,
		run: func(name string, args ...string) ([]byte, error) {
			if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
				return []byte("ok"), nil // detectBaseBranch
			}
			if len(args) >= 1 && args[0] == "diff" {
				if len(args) >= 2 && args[1] == "--name-status" {
					return nil, fmt.Errorf("name-status error")
				}
				return []byte("diff content"), nil
			}
			return nil, fmt.Errorf("unexpected")
		},
	}
	_, err := client.GetPRDiff("ref")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git diff --name-status failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostComments_RequiresGH(t *testing.T) {
	client := &Client{useGH: false}
	pr := &PR{Number: "1", HeadSHA: "abc123"}
	err := client.PostComments(pr, []Suggestion{{File: "a.go", Line: 1, Body: "test", Role: "test"}})
	if err == nil {
		t.Fatal("expected error when gh is not available")
	}
	if !strings.Contains(err.Error(), "requires the gh CLI") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostComments_RequiresHeadSHA(t *testing.T) {
	client := &Client{useGH: true}
	pr := &PR{Number: "1", HeadSHA: ""}
	err := client.PostComments(pr, []Suggestion{{File: "a.go", Line: 1, Body: "test", Role: "test"}})
	if err == nil {
		t.Fatal("expected error when HeadSHA is empty")
	}
	if !strings.Contains(err.Error(), "HEAD SHA") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPostComments_EmptySuggestions(t *testing.T) {
	client := &Client{useGH: true}
	pr := &PR{Number: "1", HeadSHA: "abc123"}
	err := client.PostComments(pr, []Suggestion{})
	if err != nil {
		t.Errorf("expected no error for empty suggestions, got: %v", err)
	}
}

func TestPostComments_Success(t *testing.T) {
	execCalls := 0
	client := &Client{
		useGH: true,
		exec: func(name string, args ...string) error {
			execCalls++
			return nil
		},
	}
	pr := &PR{Number: "1", HeadSHA: "abc123"}
	suggestions := []Suggestion{
		{File: "a.go", Line: 10, Body: "fix this", Role: "sentinel"},
		{File: "b.go", Line: 20, Body: "fix that", Role: "editor"},
	}
	err := client.PostComments(pr, suggestions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if execCalls != 2 {
		t.Errorf("expected 2 exec calls, got %d", execCalls)
	}
}

func TestPostComments_PartialFailure(t *testing.T) {
	callCount := 0
	client := &Client{
		useGH: true,
		exec: func(name string, args ...string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("api error")
			}
			return nil
		},
	}
	pr := &PR{Number: "1", HeadSHA: "abc123"}
	suggestions := []Suggestion{
		{File: "a.go", Line: 10, Body: "ok", Role: "test"},
		{File: "b.go", Line: 20, Body: "fail", Role: "test"},
		{File: "c.go", Line: 30, Body: "ok", Role: "test"},
	}
	err := client.PostComments(pr, suggestions)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	if !strings.Contains(err.Error(), "1/3") {
		t.Errorf("expected '1/3' in error, got: %v", err)
	}
	// Should have tried all 3, not stopped at the first error.
	if callCount != 3 {
		t.Errorf("expected 3 calls (continue on error), got %d", callCount)
	}
}

func TestPR_Fields(t *testing.T) {
	pr := &PR{
		Number: "42", Title: "Test", Body: "desc",
		Diff: "+ line", HeadSHA: "abc", Files: []FileChange{{Path: "a.go"}},
	}
	if pr.Number != "42" || pr.HeadSHA != "abc" || len(pr.Files) != 1 {
		t.Error("fields not set")
	}
}

func TestSuggestion_Fields(t *testing.T) {
	s := Suggestion{File: "a.go", Line: 10, Body: "fix", Role: "sentinel"}
	if s.File != "a.go" || s.Line != 10 || s.Role != "sentinel" {
		t.Errorf("fields not set: %+v", s)
	}
}

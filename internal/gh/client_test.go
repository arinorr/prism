package gh

import "testing"

func TestGitStatusToString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"A", "added"},
		{"M", "modified"},
		{"D", "removed"},
		{"R", "renamed"},
		{"X", "modified"}, // unknown defaults to modified
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
	// This test runs in a git repo, so it should return a valid branch.
	branch := detectBaseBranch()
	if branch != "main" && branch != "master" {
		t.Errorf("expected 'main' or 'master', got %q", branch)
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
}

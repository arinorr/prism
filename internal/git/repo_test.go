package git

import (
	"os/exec"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantName  string
	}{
		{"HTTPS", "https://github.com/arinorr/prism.git", "arinorr", "prism"},
		{"HTTPS no .git", "https://github.com/arinorr/prism", "arinorr", "prism"},
		{"SSH", "git@github.com:arinorr/prism.git", "arinorr", "prism"},
		{"SSH no .git", "git@github.com:arinorr/prism", "arinorr", "prism"},
		{"HTTPS with trailing slash", "https://github.com/owner/repo.git/", "repo.git", ""},
		{"empty", "", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			owner, name := ParseRemoteURL(tt.url)
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestOpenWith_MockRunner(t *testing.T) {
	t.Parallel()

	mockRun := func(name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 0 && args[0] == "rev-parse" {
			return []byte("/tmp/fake-repo\n"), nil
		}
		return nil, nil
	}

	repo, err := OpenWith(mockRun)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Root() != "/tmp/fake-repo" {
		t.Errorf("Root() = %q, want /tmp/fake-repo", repo.Root())
	}
}

func TestRepo_RemoteName_Mock(t *testing.T) {
	t.Parallel()

	repo := &Repo{
		root: "/tmp/fake",
		run: func(name string, args ...string) ([]byte, error) {
			return []byte("https://github.com/arinorr/prism.git\n"), nil
		},
	}

	if got := repo.RemoteName(); got != "prism" {
		t.Errorf("RemoteName() = %q, want prism", got)
	}
}

func TestRepo_RemoteOwnerRepo_Mock(t *testing.T) {
	t.Parallel()

	repo := &Repo{
		root: "/tmp/fake",
		run: func(name string, args ...string) ([]byte, error) {
			return []byte("git@github.com:arinorr/whetstone.git\n"), nil
		},
	}

	if got := repo.RemoteOwnerRepo(); got != "arinorr/whetstone" {
		t.Errorf("RemoteOwnerRepo() = %q, want arinorr/whetstone", got)
	}
}

func TestRepo_DefaultBranch_Mock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		branches  map[string]bool
		want      string
	}{
		{"main exists", map[string]bool{"main": true}, "main"},
		{"only master", map[string]bool{"master": true}, "master"},
		{"neither", map[string]bool{}, "main"},
		{"both", map[string]bool{"main": true, "master": true}, "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &Repo{
				root: "/tmp/fake",
				run: func(name string, args ...string) ([]byte, error) {
					if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--verify" {
						if tt.branches[args[2]] {
							return []byte("abc123\n"), nil
						}
						return nil, &exec.ExitError{}
					}
					return nil, nil
				},
			}
			if got := repo.DefaultBranch(); got != tt.want {
				t.Errorf("DefaultBranch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusToString(t *testing.T) {
	t.Parallel()
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
		if got := statusToString(tt.input); got != tt.want {
			t.Errorf("statusToString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

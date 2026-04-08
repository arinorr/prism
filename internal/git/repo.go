// Package git provides local git repository operations.
// This package owns all direct git CLI interactions. The gh package
// handles GitHub CLI operations; this package handles local git.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// commandRunner executes a command and returns its output.
type commandRunner func(name string, args ...string) ([]byte, error)

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output() // #nosec G204 -- commands are always "git" with safe args
}

// Repo represents a local git repository.
type Repo struct {
	root string
	run  commandRunner
}

// Open finds the git repo root from the current working directory.
// Returns an error if not inside a git repo.
func Open() (*Repo, error) {
	return OpenWith(defaultRunner)
}

// OpenWith opens a repo using a custom command runner (for testing).
func OpenWith(run commandRunner) (*Repo, error) {
	out, err := run("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	return &Repo{
		root: strings.TrimSpace(string(out)),
		run:  run,
	}, nil
}

// OpenAt opens a repo at a specific path.
func OpenAt(path string) (*Repo, error) {
	return &Repo{
		root: path,
		run:  defaultRunner,
	}, nil
}

// Root returns the absolute path to the repo root.
func (r *Repo) Root() string {
	return r.root
}

// RemoteName extracts the repository name from the origin remote URL.
// Handles both SSH (git@github.com:owner/repo.git) and HTTPS
// (https://github.com/owner/repo.git). Falls back to "unknown".
func (r *Repo) RemoteName() string {
	_, name := r.remoteOwnerAndName()
	return name
}

// RemoteOwnerRepo returns "owner/repo" from the origin remote URL.
// Falls back to "unknown/unknown" if the remote can't be parsed.
func (r *Repo) RemoteOwnerRepo() string {
	owner, name := r.remoteOwnerAndName()
	return owner + "/" + name
}

func (r *Repo) remoteOwnerAndName() (owner, name string) {
	out, err := r.run("git", "remote", "get-url", "origin")
	if err != nil {
		return "unknown", "unknown"
	}
	return ParseRemoteURL(strings.TrimSpace(string(out)))
}

// ParseRemoteURL extracts owner and repo name from a git remote URL.
// Handles SSH (git@github.com:owner/repo.git) and HTTPS
// (https://github.com/owner/repo.git).
func ParseRemoteURL(url string) (owner, name string) {
	url = strings.TrimSuffix(url, ".git")

	// HTTPS: https://github.com/owner/repo
	if idx := strings.LastIndex(url, "/"); idx != -1 {
		name = url[idx+1:]
		rest := url[:idx]
		if idx2 := strings.LastIndex(rest, "/"); idx2 != -1 {
			owner = rest[idx2+1:]
			return owner, name
		}
	}

	// SSH: git@github.com:owner/repo
	if idx := strings.LastIndex(url, ":"); idx != -1 {
		path := url[idx+1:]
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
		return "unknown", path
	}

	return "unknown", "unknown"
}

// DefaultBranch returns "main" or "master" (whichever exists).
// Defaults to "main" if neither can be verified.
func (r *Repo) DefaultBranch() string {
	for _, branch := range []string{"main", "master"} {
		if _, err := r.run("git", "rev-parse", "--verify", branch); err == nil {
			return branch
		}
	}
	return "main"
}

// HeadSHA returns the current HEAD commit SHA.
// Returns empty string if it can't be determined.
func (r *Repo) HeadSHA() string {
	out, err := r.run("git", "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Diff returns the unified diff between the default branch and HEAD.
func (r *Repo) Diff() (string, error) {
	base := r.DefaultBranch()
	out, err := r.run("git", "diff", base+"...HEAD")
	if err != nil {
		return "", fmt.Errorf("git diff %s...HEAD failed: %w", base, err)
	}
	return string(out), nil
}

// FileChange represents a single file's changes.
type FileChange struct {
	Path   string
	Status string // added, modified, removed, renamed
}

// FileChanges returns the list of changed files between default branch and HEAD.
func (r *Repo) FileChanges() ([]FileChange, error) {
	base := r.DefaultBranch()
	out, err := r.run("git", "diff", "--name-status", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status failed: %w", err)
	}

	var files []FileChange
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, FileChange{
				Path:   parts[1],
				Status: statusToString(parts[0]),
			})
		}
	}
	return files, nil
}

func statusToString(s string) string {
	switch s {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "removed"
	case "R":
		return "renamed"
	default:
		return "modified"
	}
}

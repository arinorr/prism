package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// PR holds the metadata and diff for a pull request.
type PR struct {
	Number  string
	Repo    string // repository name (e.g., "prism")
	Title   string
	Body    string
	Diff    string
	HeadSHA string
	Files   []FileChange
}

// FileChange represents a single file's changes in the PR.
type FileChange struct {
	Path   string
	Status string // added, modified, removed, renamed
}

// Suggestion is a review comment tied to a specific file and line.
type Suggestion struct {
	File string
	Line int
	Body string
	Role string // which agent produced this
}

// commandRunner executes a command and returns its output.
// Defaults to exec.Command(...).Output() but can be replaced in tests.
type commandRunner func(name string, args ...string) ([]byte, error)

func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output() // #nosec G204 -- command name is always "gh" or "git", args validated by ValidatePRRef
}

func defaultRunnerNoOutput(name string, args ...string) error {
	return exec.Command(name, args...).Run() // #nosec G204 -- command name is always "gh", args validated by ValidatePRRef
}

// Client wraps GitHub CLI interactions.
type Client struct {
	useGH bool
	run   commandRunner
	exec  func(name string, args ...string) error
}

// NewClient creates a new GitHub client, preferring gh if available.
func NewClient() (*Client, error) {
	_, err := exec.LookPath("gh")
	return &Client{useGH: err == nil, run: defaultRunner, exec: defaultRunnerNoOutput}, nil
}

// prRefPattern matches valid PR references: numbers, GitHub URLs, or branch-like names.
var prRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)

// ValidatePRRef checks that a PR reference is safe to pass to external commands.
func ValidatePRRef(prRef string) error {
	if prRef == "" {
		return fmt.Errorf("PR reference cannot be empty")
	}
	if strings.HasPrefix(prRef, "-") {
		return fmt.Errorf("invalid PR reference %q: cannot start with a dash", prRef)
	}
	if !prRefPattern.MatchString(prRef) {
		return fmt.Errorf("invalid PR reference %q: contains disallowed characters", prRef)
	}
	return nil
}

// GetPRDiff fetches the PR diff and metadata.
func (c *Client) GetPRDiff(prRef string) (*PR, error) {
	if err := ValidatePRRef(prRef); err != nil {
		return nil, fmt.Errorf("invalid PR reference: %w", err)
	}
	if !c.useGH {
		return c.getPRDiffGit(prRef)
	}
	return c.getPRDiffGH(prRef)
}

func (c *Client) getPRDiffGH(prRef string) (*PR, error) {
	// Get PR metadata and files in a single call.
	out, err := c.run("gh", "pr", "view", prRef, "--json", "number,title,body,headRefOid,files,headRepository")
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %w", err)
	}

	var meta struct {
		Number     int    `json:"number"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		HeadRefOid string `json:"headRefOid"`
		Files      []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
		HeadRepository struct {
			Name string `json:"name"`
		} `json:"headRepository"`
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse PR metadata: %w", err)
	}

	// Get the diff (no JSON equivalent for this).
	diff, err := c.run("gh", "pr", "diff", prRef)
	if err != nil {
		return nil, fmt.Errorf("gh pr diff failed: %w", err)
	}

	files := make([]FileChange, len(meta.Files))
	for i, f := range meta.Files {
		files[i] = FileChange{
			Path:   f.Path,
			Status: "modified",
		}
	}

	repo := meta.HeadRepository.Name
	if repo == "" {
		repo = c.detectRepoName() // fallback for older gh versions
	}

	return &PR{
		Number:  fmt.Sprintf("%d", meta.Number),
		Repo:    repo,
		Title:   meta.Title,
		Body:    meta.Body,
		Diff:    string(diff),
		HeadSHA: meta.HeadRefOid,
		Files:   files,
	}, nil
}

func (c *Client) getPRDiffGit(prRef string) (*PR, error) {
	// Fallback: use git diff against main/master.
	// This is a simplified fallback — assumes the PR branch is checked out.
	base := c.detectBaseBranch()

	diff, err := c.run("git", "diff", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	// Get changed file list.
	filesOut, err := c.run("git", "diff", "--name-status", base+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status failed: %w", err)
	}

	var files []FileChange
	for _, line := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, FileChange{
				Path:   parts[1],
				Status: gitStatusToString(parts[0]),
			})
		}
	}

	// Get HEAD commit SHA.
	headSHA := ""
	if shaOut, shaErr := c.run("git", "rev-parse", "HEAD"); shaErr == nil {
		headSHA = strings.TrimSpace(string(shaOut))
	}

	return &PR{
		Number:  prRef,
		Repo:    c.detectRepoName(),
		Title:   fmt.Sprintf("(local) Changes vs %s", base),
		Diff:    string(diff),
		HeadSHA: headSHA,
		Files:   files,
	}, nil
}

// PostComments posts inline review comments on a PR.
func (c *Client) PostComments(pr *PR, suggestions []Suggestion) error {
	if !c.useGH {
		return fmt.Errorf("posting comments requires the gh CLI")
	}

	if pr.HeadSHA == "" {
		return fmt.Errorf("cannot post comments: HEAD SHA is not available")
	}

	var errs []error
	for _, s := range suggestions {
		body := fmt.Sprintf("**[%s]** %s", s.Role, s.Body)
		err := c.exec("gh", "api",
			fmt.Sprintf("repos/{owner}/{repo}/pulls/%s/comments", pr.Number),
			"-f", fmt.Sprintf("body=%s", body),
			"-f", fmt.Sprintf("path=%s", s.File),
			"-F", fmt.Sprintf("line=%d", s.Line),
			"-f", fmt.Sprintf("commit_id=%s", pr.HeadSHA),
		)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to post comment on %s:%d: %w", s.File, s.Line, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to post %d/%d comments: %w", len(errs), len(suggestions), errs[0])
	}
	return nil
}

// detectRepoName returns the repository name from the git remote URL.
// Falls back to "unknown" if the remote can't be parsed.
func (c *Client) detectRepoName() string {
	out, err := c.run("git", "remote", "get-url", "origin")
	if err != nil {
		return "unknown"
	}
	url := strings.TrimSpace(string(out))
	// Handle SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
	url = strings.TrimSuffix(url, ".git")
	if idx := strings.LastIndex(url, "/"); idx != -1 {
		return url[idx+1:]
	}
	if idx := strings.LastIndex(url, ":"); idx != -1 {
		return url[idx+1:]
	}
	return "unknown"
}

func (c *Client) detectBaseBranch() string {
	for _, branch := range []string{"main", "master"} {
		if _, err := c.run("git", "rev-parse", "--verify", branch); err == nil {
			return branch
		}
	}
	return "main"
}

func gitStatusToString(status string) string {
	switch status {
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

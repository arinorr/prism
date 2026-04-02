package gh

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// PR holds the metadata and diff for a pull request.
type PR struct {
	Number  string
	Title   string
	Body    string
	Diff    string
	HeadSHA string
	Files   []FileChange
}

// FileChange represents a single file's changes in the PR.
type FileChange struct {
	Path   string
	Patch  string
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
	return exec.Command(name, args...).Output()
}

func defaultRunnerNoOutput(name string, args ...string) error {
	return exec.Command(name, args...).Run()
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

// GetPRDiff fetches the PR diff and metadata.
func (c *Client) GetPRDiff(prRef string) (*PR, error) {
	if !c.useGH {
		return c.getPRDiffGit(prRef)
	}
	return c.getPRDiffGH(prRef)
}

func (c *Client) getPRDiffGH(prRef string) (*PR, error) {
	// Get PR metadata.
	out, err := c.run("gh", "pr", "view", prRef, "--json", "number,title,body,headRefOid")
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %w", err)
	}

	var meta struct {
		Number     int    `json:"number"`
		Title      string `json:"title"`
		Body       string `json:"body"`
		HeadRefOid string `json:"headRefOid"`
	}
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse PR metadata: %w", err)
	}

	// Get the diff.
	diff, err := c.run("gh", "pr", "diff", prRef)
	if err != nil {
		return nil, fmt.Errorf("gh pr diff failed: %w", err)
	}

	// Get changed files.
	filesOut, err := c.run("gh", "pr", "view", prRef, "--json", "files")
	if err != nil {
		return nil, fmt.Errorf("gh pr view files failed: %w", err)
	}

	var filesData struct {
		Files []struct {
			Path      string `json:"path"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
	}
	if err := json.Unmarshal(filesOut, &filesData); err != nil {
		return nil, fmt.Errorf("failed to parse files: %w", err)
	}

	files := make([]FileChange, len(filesData.Files))
	for i, f := range filesData.Files {
		files[i] = FileChange{
			Path:   f.Path,
			Status: "modified",
		}
	}

	pr := &PR{
		Number:  fmt.Sprintf("%d", meta.Number),
		Title:   meta.Title,
		Body:    meta.Body,
		Diff:    string(diff),
		HeadSHA: meta.HeadRefOid,
		Files:   files,
	}

	return pr, nil
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

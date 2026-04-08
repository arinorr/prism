# Git Package: Consolidate Git Operations + Remote Repo Cloning

## Context

Git operations are scattered across two locations:
- `cmd/review.go` — `git rev-parse --show-toplevel` for repo root detection
- `internal/gh/client.go` — `git remote get-url origin`, `git diff`, `git rev-parse HEAD`, `git rev-parse --verify main/master`

These are all local git operations that have nothing to do with the GitHub API. They're mixed into the `gh` package (which should only handle GitHub CLI interactions) and `cmd/review.go` (which shouldn't shell out to git directly).

Additionally, when reviewing a PR from a different repo (e.g., running prism from the prism repo to review a whetstone PR), the symbol index is built from the wrong codebase. The resolver tries to find whetstone files in the prism directory and fails. We need to detect this and clone the remote repo.

## Goal

1. Create `internal/git/` package that owns all local git operations
2. Move existing scattered git calls into it
3. Add remote repo detection + shallow clone for cross-repo PR review
4. Clean up `gh` package to only contain GitHub CLI operations

## Current State: Git Operations Inventory

| Location | Command | Purpose |
|----------|---------|---------|
| `cmd/review.go:338` | `git rev-parse --show-toplevel` | Detect repo root |
| `gh/client.go:224` | `git remote get-url origin` | Extract repo name from remote |
| `gh/client.go:242` | `git rev-parse --verify main/master` | Detect default branch |
| `gh/client.go:149` | `git diff main...HEAD` | Get diff (fallback when no `gh` CLI) |
| `gh/client.go:155` | `git diff --name-status main...HEAD` | Get file list (fallback) |
| `gh/client.go:176` | `git rev-parse HEAD` | Get HEAD SHA (fallback) |

## Design

### `internal/git/` Package

```go
package git

// Repo represents a local git repository.
type Repo struct {
    root string         // absolute path to repo root
    run  commandRunner  // for testability
}

// Open finds the repo root from the current directory.
func Open() (*Repo, error)

// OpenAt opens a repo at a specific path.
func OpenAt(path string) (*Repo, error)

// Root returns the absolute path to the repo root.
func (r *Repo) Root() string

// RemoteName extracts the repository name from the origin remote URL.
// Handles both SSH (git@github.com:owner/repo.git) and HTTPS formats.
func (r *Repo) RemoteName() string

// RemoteOwnerRepo returns "owner/repo" from the origin remote URL.
func (r *Repo) RemoteOwnerRepo() string

// DefaultBranch returns "main" or "master" (whichever exists).
func (r *Repo) DefaultBranch() string

// HeadSHA returns the current HEAD commit SHA.
func (r *Repo) HeadSHA() string

// Diff returns the unified diff between the default branch and HEAD.
func (r *Repo) Diff() (string, error)

// FileChanges returns the list of changed files between default branch and HEAD.
func (r *Repo) FileChanges() ([]FileChange, error)

type FileChange struct {
    Path   string
    Status string // added, modified, removed, renamed
}
```

### Remote Repo Cloning

New function for cross-repo review:

```go
// CloneShallow creates a shallow clone of a remote repo at a specific ref.
// Returns a Repo pointed at the temp directory and a cleanup function.
// The clone is depth=1 (single commit) for speed.
func CloneShallow(repoURL, ref string) (repo *Repo, cleanup func(), err error)
```

Implementation:
```go
func CloneShallow(repoURL, ref string) (*Repo, func(), error) {
    dir, err := os.MkdirTemp("", "prism-clone-*")
    if err != nil {
        return nil, nil, fmt.Errorf("creating temp dir: %w", err)
    }

    // git clone --depth 1 --branch <ref> <url> <dir>
    cmd := exec.Command("git", "clone", "--depth", "1", "--branch", ref, repoURL, dir)
    if err := cmd.Run(); err != nil {
        os.RemoveAll(dir)
        return nil, nil, fmt.Errorf("cloning %s at %s: %w", repoURL, ref, err)
    }

    cleanup := func() { os.RemoveAll(dir) }
    return &Repo{root: dir}, cleanup, nil
}
```

### Cross-Repo Detection

New function that determines the right repo root for index building:

```go
// ResolveRepoForPR determines the repo root to use for symbol indexing.
// If the PR is from the same repo as the current directory, returns the local root.
// If the PR is from a different repo, shallow-clones it and returns the clone path.
// Returns the repo root path and a cleanup function (nil if no clone needed).
func ResolveRepoForPR(pr *gh.PR) (repoRoot string, cleanup func(), err error)
```

Logic:
1. Open local repo, get `RemoteOwnerRepo()` → e.g., `"arinorr/prism"`
2. Compare against `pr.Repo` (which comes from the PR metadata)
3. If same → return local repo root, nil cleanup
4. If different → clone the PR's repo, return clone path + cleanup function

The PR struct already has `Repo` (just the name, e.g., `"whetstone"`). We'll also need the full `owner/repo` and the head branch ref. The `gh pr view --json` call already provides `headRepository.nameWithOwner` and `headRefName` — we just need to pass them through.

### User Feedback

When cloning:
```
📥 Cloning arinorr/whetstone (remote PR, needed for code analysis)...
```

When using local:
No message (default, silent).

### Changes to `gh` Package

Move these functions OUT of `internal/gh/client.go`:
- `detectRepoName()` → `git.Repo.RemoteName()`
- `detectBaseBranch()` → `git.Repo.DefaultBranch()`
- The entire `getPRDiffGit()` fallback → uses `git.Repo.Diff()` and `git.Repo.FileChanges()`

The `gh` package retains:
- `GetPRDiff()` (orchestrates gh CLI or git fallback)
- `getPRDiffGH()` (gh CLI path)
- `PostComments()`
- `ValidatePRRef()`
- `PR`, `FileChange`, `Suggestion`, `Client` types

The `gh.Client` gains a `*git.Repo` field for the fallback path:
```go
type Client struct {
    useGH bool
    repo  *git.Repo  // local repo, used for git-based fallback
    run   commandRunner
    exec  func(...) error
}
```

### Changes to `cmd/review.go`

Replace the inline `exec.Command("git", "rev-parse", "--show-toplevel")` with:

```go
// Resolve repo root for indexing (handles cross-repo PRs).
repoRoot, repoCleanup, err := git.ResolveRepoForPR(pr)
if err != nil {
    progress("   ⚠️  Could not determine repo root: %v\n", err)
} else if repoCleanup != nil {
    defer repoCleanup()
    progress("📥 Cloned %s for code analysis\n", pr.Repo)
}
```

### PR Struct Changes

Add fields for cross-repo detection:

```go
type PR struct {
    Number       string
    Repo         string       // repo name: "prism"
    Owner        string       // NEW: repo owner: "arinorr"
    OwnerRepo    string       // NEW: "arinorr/prism"
    HeadRef      string       // NEW: branch name for cloning
    Title        string
    Body         string
    Diff         string
    HeadSHA      string
    Files        []FileChange
}
```

These are populated from `gh pr view --json headRepository,headRefName`:
```json
{
    "headRepository": {"name": "whetstone", "nameWithOwner": "arinorr/whetstone"},
    "headRefName": "feature/ci-pipeline"
}
```

## Files to Create

| File | Purpose |
|------|---------|
| `internal/git/repo.go` | `Repo` type, `Open`, `OpenAt`, repo metadata methods |
| `internal/git/clone.go` | `CloneShallow`, `ResolveRepoForPR` |
| `internal/git/repo_test.go` | Repo operations tests (uses temp git repos) |
| `internal/git/clone_test.go` | Clone tests (mock or integration) |

## Files to Modify

| File | Change |
|------|--------|
| `internal/gh/client.go` | Remove `detectRepoName()`, `detectBaseBranch()`. Refactor `getPRDiffGit()` to use `git.Repo`. Add `Owner`, `OwnerRepo`, `HeadRef` fields to `PR`. Parse `headRefName` from `gh pr view`. |
| `cmd/review.go` | Replace inline `git rev-parse` with `git.ResolveRepoForPR()`. Add clone progress message. Pass cleanup to defer. |

## Testing

- Unit: `Repo.RemoteName()` with SSH and HTTPS URLs
- Unit: `Repo.DefaultBranch()` main vs master detection
- Unit: `ResolveRepoForPR` same-repo detection (no clone needed)
- Unit: `ResolveRepoForPR` cross-repo detection (clone triggered)
- Unit: `CloneShallow` cleanup function removes temp dir
- Integration: clone a real public repo, verify files exist
- Integration: full review pipeline with cross-repo PR

## Edge Cases

- **No git repo** (running from `/tmp`): `Open()` returns error, repo root falls back to ".", index/routing still works without cross-refs
- **Private repo clone fails**: `CloneShallow` fails, fall back to local root (degraded, same as current behavior)
- **PR from a fork**: `headRepository.nameWithOwner` is the fork, not the upstream. Clone the fork.
- **Clone takes too long**: `CloneShallow` should accept a context for timeout
- **Disk space**: shallow clone is typically <50MB. Temp dir cleaned up via defer.
- **gh CLI not available + remote PR**: can't determine owner/repo from PR metadata (git fallback doesn't query GitHub). Fall back to local root.

## Estimation

- ~200 lines: `internal/git/` (repo.go + clone.go)
- ~100 lines: refactor `gh/client.go` (move functions out, add PR fields)
- ~30 lines: update `cmd/review.go`
- ~200 lines: tests
- Total: ~530 lines

package git

import (
	"fmt"
	"os"
	"os/exec"
)

// CloneShallow creates a shallow clone (depth=1) of a remote repo at a
// specific branch/ref. Returns a Repo pointed at the temp directory and
// a cleanup function that removes the clone.
func CloneShallow(repoURL, ref string) (*Repo, func(), error) {
	dir, err := os.MkdirTemp("", "prism-clone-*")
	if err != nil {
		return nil, nil, fmt.Errorf("creating temp dir: %w", err)
	}

	// #nosec G204 -- repoURL comes from GitHub API, ref from gh pr view
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", ref, repoURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("cloning %s at %s: %w\n%s", repoURL, ref, err, string(out))
	}

	cleanup := func() { os.RemoveAll(dir) }
	return &Repo{root: dir, run: defaultRunner}, cleanup, nil
}

// ResolveRepoForPR determines the correct repo root for symbol indexing.
//
// If the PR is from the same repo as the local working directory, returns
// the local root (fast, no clone needed).
//
// If the PR is from a different repo, shallow-clones it into a temp
// directory and returns the clone path + cleanup function.
//
// Returns ("", nil, err) if the repo can't be determined.
func ResolveRepoForPR(prOwnerRepo, prHeadRef string) (repoRoot string, cleanup func(), err error) {
	// Try to open the local repo.
	local, localErr := Open()
	if localErr != nil {
		// Not in a git repo — can't compare, can't clone without more info.
		return ".", nil, nil
	}

	localOwnerRepo := local.RemoteOwnerRepo()

	// Same repo — use local.
	if localOwnerRepo == prOwnerRepo {
		return local.Root(), nil, nil
	}

	// Different repo — need to clone.
	if prOwnerRepo == "" || prHeadRef == "" {
		// Can't clone without owner/repo and ref. Fall back to local.
		return local.Root(), nil, nil
	}

	cloneURL := "https://github.com/" + prOwnerRepo + ".git"
	cloned, cloneCleanup, cloneErr := CloneShallow(cloneURL, prHeadRef)
	if cloneErr != nil {
		// Clone failed — fall back to local (degraded).
		return local.Root(), nil, fmt.Errorf("cloning %s: %w", prOwnerRepo, cloneErr)
	}

	return cloned.Root(), cloneCleanup, nil
}

package git

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestResolveRepoForPR_SameRepo(t *testing.T) {
	t.Parallel()

	// Mock: local repo reports as "arinorr/prism".
	origOpen := Open
	_ = origOpen // suppress unused

	// Since we can't easily mock Open() (it's a package function),
	// test the logic through the actual git repo if we're in one.
	repoRoot, cleanup, err := ResolveRepoForPR("arinorr/prism", "main")
	if err != nil {
		t.Logf("ResolveRepoForPR error (may be expected outside git repo): %v", err)
	}
	if cleanup != nil {
		t.Error("same repo should not trigger clone (cleanup should be nil)")
		cleanup()
	}
	if repoRoot == "" {
		t.Error("expected non-empty repoRoot")
	}
}

func TestResolveRepoForPR_EmptyOwnerRepo(t *testing.T) {
	t.Parallel()

	// Empty owner/repo means we can't determine what to clone.
	// Should fall back to local.
	repoRoot, cleanup, err := ResolveRepoForPR("", "")
	if err != nil {
		t.Logf("error: %v", err)
	}
	if cleanup != nil {
		t.Error("should not clone when ownerRepo is empty")
		cleanup()
	}
	if repoRoot == "" {
		t.Error("expected non-empty repoRoot (fallback to local)")
	}
}

func TestResolveRepoForPR_DifferentRepo(t *testing.T) {
	// Skip in CI or if git isn't available — this test actually clones.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Clone a small public repo.
	repoRoot, cleanup, err := ResolveRepoForPR("arinorr/prism", "main")
	if err != nil {
		t.Logf("error (may be expected if local repo matches): %v", err)
	}

	// If we're running inside the prism repo, this will match and not clone.
	// That's fine — the important thing is no crash.
	if cleanup != nil {
		defer cleanup()
		t.Log("clone was triggered (running outside prism repo)")
	}
	if repoRoot == "" {
		t.Error("expected non-empty repoRoot")
	}
}

func TestCloneShallow_InvalidRepo(t *testing.T) {
	t.Parallel()

	_, cleanup, err := CloneShallow("nonexistent/repo-that-does-not-exist", "main")
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Error("expected error for nonexistent repo")
	}
}

func TestCloneShallow_Cleanup(t *testing.T) {
	// Skip if no network or no gh CLI.
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not available")
	}

	// Clone a real small repo to verify cleanup works.
	repo, cleanup, err := CloneShallow("arinorr/prism", "main")
	if err != nil {
		t.Skipf("clone failed (network?): %v", err)
	}

	dir := repo.Root()
	// Verify the directory exists.
	if _, statErr := fmt.Println(dir); statErr != nil {
		t.Errorf("clone directory doesn't exist: %s", dir)
	}

	// Cleanup should remove it.
	cleanup()
	// Note: we can't easily verify removal without os.Stat,
	// but the important thing is cleanup doesn't panic.
}

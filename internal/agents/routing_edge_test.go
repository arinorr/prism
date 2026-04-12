package agents

import (
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestBuildReviewContext_NilIndex(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{
		Diff:  "diff --git a/main.go b/main.go\n+package main\n",
		Files: []gh.FileChange{{Path: "main.go", Status: "added"}},
	}

	rctx := BuildReviewContext(pr, nil, nil)
	if rctx == nil {
		t.Fatal("expected non-nil ReviewContext")
		return
	}
	if rctx.Index != nil {
		t.Error("expected nil Index")
	}
	if rctx.Resolver != nil {
		t.Error("expected nil Resolver")
	}
	if rctx.ChangeMap != nil {
		t.Error("expected nil ChangeMap when index is nil")
	}
	if len(rctx.Files) == 0 {
		t.Error("expected files to be classified even without index")
	}
}

func TestBuildReviewContext_FilesNotInDiff(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{
		Diff: "diff --git a/main.go b/main.go\n+package main\n",
		Files: []gh.FileChange{
			{Path: "main.go", Status: "modified"},
			{Path: "deleted.go", Status: "removed"}, // no diff section
		},
	}

	rctx := BuildReviewContext(pr, nil, nil)

	paths := make(map[string]bool)
	for _, f := range rctx.Files {
		paths[f.Path] = true
	}
	if !paths["main.go"] {
		t.Error("expected main.go in files")
	}
	if !paths["deleted.go"] {
		t.Error("expected deleted.go in files (from pr.Files)")
	}
}

func TestBuildReviewContext_EmptyPR(t *testing.T) {
	t.Parallel()
	pr := &gh.PR{}

	rctx := BuildReviewContext(pr, nil, nil)
	if rctx == nil {
		t.Fatal("expected non-nil ReviewContext")
		return
	}
	if len(rctx.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(rctx.Files))
	}
}

func TestRoutingSummary_EmptyRoles(t *testing.T) {
	t.Parallel()
	files := []ClassifiedFile{{Path: "main.go", Category: PRCategoryCode}}
	got := RoutingSummary(files, nil)
	// No roles → no agent names per category. Should still show file counts.
	if got == "" {
		t.Error("expected non-empty summary even with no roles")
	}
}

func TestRoutingSummary_NoFiles(t *testing.T) {
	t.Parallel()
	got := RoutingSummary(nil, AllRoles)
	if got != "" {
		t.Errorf("expected empty summary for no files, got %q", got)
	}
}

func TestRoutingSummary_MixedFiles(t *testing.T) {
	t.Parallel()
	files := []ClassifiedFile{
		{Path: "main.go", Category: PRCategoryCode},
		{Path: "README.md", Category: PRCategoryDocs},
		{Path: "Dockerfile", Category: PRCategoryConfig},
	}

	got := RoutingSummary(files, AllRoles)

	if got == "" {
		t.Error("expected non-empty summary")
	}
}

func TestReviewContext_SymbolStatus_NilContext(t *testing.T) {
	t.Parallel()
	var rctx *ReviewContext
	got := rctx.SymbolStatus("file.go", "Func")
	if got != SymbolExisting {
		t.Errorf("nil ReviewContext should return SymbolExisting, got %s", got)
	}
}

func TestReviewContext_SymbolStatus_NilChangeMap(t *testing.T) {
	t.Parallel()
	rctx := &ReviewContext{} // ChangeMap is nil
	got := rctx.SymbolStatus("file.go", "Func")
	if got != SymbolExisting {
		t.Errorf("nil ChangeMap should return SymbolExisting, got %s", got)
	}
}

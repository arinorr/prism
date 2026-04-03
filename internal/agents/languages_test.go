package agents

import (
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestDetectLanguages_TypeScript(t *testing.T) {
	files := []gh.FileChange{
		{Path: "src/app.ts"},
		{Path: "src/index.tsx"},
	}
	got := DetectLanguages(files)
	if len(got) != 1 || got[0] != "typescript" {
		t.Errorf("expected [typescript], got %v", got)
	}
}

func TestDetectLanguages_Go(t *testing.T) {
	files := []gh.FileChange{
		{Path: "main.go"},
		{Path: "internal/foo.go"},
	}
	got := DetectLanguages(files)
	if len(got) != 1 || got[0] != "go" {
		t.Errorf("expected [go], got %v", got)
	}
}

func TestDetectLanguages_Mixed(t *testing.T) {
	files := []gh.FileChange{
		{Path: "main.go"},
		{Path: "src/app.ts"},
	}
	got := DetectLanguages(files)
	if len(got) != 2 || got[0] != "go" || got[1] != "typescript" {
		t.Errorf("expected [go typescript], got %v", got)
	}
}

func TestDetectLanguages_AllJSVariants(t *testing.T) {
	files := []gh.FileChange{
		{Path: "a.ts"},
		{Path: "b.tsx"},
		{Path: "c.js"},
		{Path: "d.jsx"},
		{Path: "e.mjs"},
		{Path: "f.cjs"},
	}
	got := DetectLanguages(files)
	if len(got) != 1 || got[0] != "typescript" {
		t.Errorf("expected [typescript] (deduplicated), got %v", got)
	}
}

func TestDetectLanguages_UnknownExtensions(t *testing.T) {
	files := []gh.FileChange{
		{Path: "README.md"},
		{Path: "Dockerfile"},
		{Path: "Makefile"},
		{Path: ".gitignore"},
	}
	got := DetectLanguages(files)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestDetectLanguages_Empty(t *testing.T) {
	got := DetectLanguages(nil)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestDetectLanguages_CaseInsensitive(t *testing.T) {
	files := []gh.FileChange{
		{Path: "App.TSX"},
		{Path: "Main.GO"},
	}
	got := DetectLanguages(files)
	if len(got) != 2 || got[0] != "go" || got[1] != "typescript" {
		t.Errorf("expected [go typescript], got %v", got)
	}
}

func TestDetectLanguages_NestedExtension(t *testing.T) {
	files := []gh.FileChange{
		{Path: "component.test.ts"},
	}
	got := DetectLanguages(files)
	if len(got) != 1 || got[0] != "typescript" {
		t.Errorf("expected [typescript], got %v", got)
	}
}

func TestLanguageSkillPath_KnowItAll(t *testing.T) {
	got := languageSkillPath("skills/know-it-all.md", "typescript")
	want := "skills/know-it-all/typescript.md"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestLanguageSkillPath_Sentinel(t *testing.T) {
	got := languageSkillPath("skills/sentinel.md", "go")
	want := "skills/sentinel/go.md"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

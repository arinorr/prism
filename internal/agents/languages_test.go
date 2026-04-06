package agents

import (
	"reflect"
	"testing"

	"github.com/arinorr/prism/internal/gh"
)

func TestDetectLanguages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files []gh.FileChange
		want  []string
	}{
		{"TypeScript", []gh.FileChange{{Path: "src/app.ts"}, {Path: "src/index.tsx"}}, []string{"typescript"}},
		{"Go", []gh.FileChange{{Path: "main.go"}, {Path: "internal/foo.go"}}, []string{"go"}},
		{"Mixed", []gh.FileChange{{Path: "main.go"}, {Path: "src/app.ts"}}, []string{"go", "typescript"}},
		{"AllJSVariants", []gh.FileChange{{Path: "a.ts"}, {Path: "b.tsx"}, {Path: "c.js"}, {Path: "d.jsx"}, {Path: "e.mjs"}, {Path: "f.cjs"}}, []string{"typescript"}},
		{"UnknownExtensions", []gh.FileChange{{Path: "README.md"}, {Path: "Dockerfile"}, {Path: "Makefile"}, {Path: ".gitignore"}}, []string{}},
		{"Empty", nil, []string{}},
		{"CaseInsensitive", []gh.FileChange{{Path: "App.TSX"}, {Path: "Main.GO"}}, []string{"go", "typescript"}},
		{"NestedExtension", []gh.FileChange{{Path: "component.test.ts"}}, []string{"typescript"}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DetectLanguages(tt.files)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageSkillPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		skill    string
		language string
		want     string
	}{
		{"KnowItAll", "skills/know-it-all.md", "typescript", "skills/know-it-all/typescript.md"},
		{"Sentinel", "skills/sentinel.md", "go", "skills/sentinel/go.md"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := languageSkillPath(tt.skill, tt.language)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

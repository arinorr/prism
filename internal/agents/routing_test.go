package agents

import (
	"testing"
)

func TestClassifyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want PRCategory
	}{
		// Directory prefix (pass 1)
		{"docs dir", "docs/guide.md", PRCategoryDocs},
		{"docs dir nested", "docs/api/reference.md", PRCategoryDocs},
		{"test dir", "test/helper.go", PRCategoryTests},
		{"test dir json", "test/fixtures/data.json", PRCategoryTests},
		{"__tests__ dir", "__tests__/app.tsx", PRCategoryTests},
		{"tests dir", "tests/integration/api.go", PRCategoryTests},
		{"github dir", ".github/workflows/ci.yml", PRCategoryConfig},
		{"github codeowners", ".github/CODEOWNERS", PRCategoryConfig},
		{"ci dir", "ci/build.sh", PRCategoryConfig},

		// Basename (pass 2)
		{"readme", "README.md", PRCategoryDocs},
		{"readme no ext", "README", PRCategoryDocs},
		{"changelog", "CHANGELOG.md", PRCategoryDocs},
		{"license", "LICENSE", PRCategoryDocs},
		{"contributing", "CONTRIBUTING.md", PRCategoryDocs},
		{"dockerfile", "Dockerfile", PRCategoryConfig},
		{"dockerfile dev", "Dockerfile.dev", PRCategoryConfig},
		{"docker-compose", "docker-compose.yml", PRCategoryConfig},
		{"docker-compose override", "docker-compose.override.yml", PRCategoryConfig},
		{"gitignore", ".gitignore", PRCategoryConfig},
		{"eslintrc", ".eslintrc.json", PRCategoryConfig},
		{"tsconfig", "tsconfig.json", PRCategoryConfig},
		{"go.mod", "go.mod", PRCategoryConfig},
		{"go.sum", "go.sum", PRCategoryConfig},
		{"env", ".env", PRCategoryConfig},
		{"env.local", ".env.local", PRCategoryConfig},
		{"env.production", ".env.production", PRCategoryConfig},

		// Suffix convention (pass 3)
		{"go test", "handler_test.go", PRCategoryTests},
		{"ts test", "handler.test.ts", PRCategoryTests},
		{"js test", "utils.test.js", PRCategoryTests},
		{"ts spec", "Component.spec.tsx", PRCategoryTests},
		{"js spec", "app.spec.js", PRCategoryTests},
		{"jsx spec", "Button.spec.jsx", PRCategoryTests},

		// Extension (pass 4)
		{"markdown", "notes.md", PRCategoryDocs},
		{"txt", "todo.txt", PRCategoryDocs},
		{"rst", "docs.rst", PRCategoryDocs},
		{"adoc", "guide.adoc", PRCategoryDocs},
		{"yml", "config.yml", PRCategoryConfig},
		{"yaml", "settings.yaml", PRCategoryConfig},
		{"toml", "config.toml", PRCategoryConfig},
		{"json", "package.json", PRCategoryConfig},
		{"ini", "settings.ini", PRCategoryConfig},

		// Code (fallthrough)
		{"go code", "main.go", PRCategoryCode},
		{"ts code", "app.ts", PRCategoryCode},
		{"tsx code", "App.tsx", PRCategoryCode},
		{"js code", "index.js", PRCategoryCode},
		{"python", "server.py", PRCategoryCode},
		{"shell", "deploy.sh", PRCategoryCode},
		{"makefile", "Makefile", PRCategoryCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifyFile(tt.path)
			if got != tt.want {
				t.Errorf("classifyFile(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestClassifyFile_DirectoryPrecedence(t *testing.T) {
	t.Parallel()
	// Directory wins over extension.
	if got := classifyFile("test/fixtures/data.json"); got != PRCategoryTests {
		t.Errorf("json in test dir: got %q, want tests", got)
	}
	if got := classifyFile("docs/schema.json"); got != PRCategoryDocs {
		t.Errorf("json in docs dir: got %q, want docs", got)
	}
}

func TestPRCategory_Valid(t *testing.T) {
	t.Parallel()
	valid := []PRCategory{PRCategoryDocs, PRCategoryTests, PRCategoryConfig, PRCategoryCode}
	for _, c := range valid {
		if !c.Valid() {
			t.Errorf("%q should be valid", c)
		}
	}
	invalid := []PRCategory{"", "unknown", "migration"}
	for _, c := range invalid {
		if c.Valid() {
			t.Errorf("%q should be invalid", c)
		}
	}
}

func TestFilterFilesForRole(t *testing.T) {
	t.Parallel()

	mixed := []ClassifiedFile{
		{Path: "handler.go", Category: PRCategoryCode, Diff: "code diff"},
		{Path: "README.md", Category: PRCategoryDocs, Diff: "docs diff"},
		{Path: "handler_test.go", Category: PRCategoryTests, Diff: "test diff"},
		{Path: "Dockerfile", Category: PRCategoryConfig, Diff: "config diff"},
	}

	tests := []struct {
		name      string
		role      Role
		wantPaths []string
	}{
		{"know-it-all sees all", Role{SeeAll: true}, []string{"handler.go", "README.md", "handler_test.go", "Dockerfile"}},
		{"architect sees code+config", Role{Relevance: []PRCategory{PRCategoryCode, PRCategoryConfig}}, []string{"handler.go", "Dockerfile"}},
		{"editor sees code+docs", Role{Relevance: []PRCategory{PRCategoryCode, PRCategoryDocs}}, []string{"handler.go", "README.md"}},
		{"optimizer sees code only", Role{Relevance: []PRCategory{PRCategoryCode}}, []string{"handler.go"}},
		{"solver sees code+tests", Role{Relevance: []PRCategory{PRCategoryCode, PRCategoryTests}}, []string{"handler.go", "handler_test.go"}},
		{"zero-value role sees nothing", Role{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			filtered := FilterFilesForRole(&tt.role, mixed)
			gotPaths := make(map[string]bool)
			for _, f := range filtered {
				gotPaths[f.Path] = true
			}
			for _, want := range tt.wantPaths {
				if !gotPaths[want] {
					t.Errorf("expected %q in filtered set", want)
				}
			}
			if len(filtered) != len(tt.wantPaths) {
				t.Errorf("got %d files, want %d", len(filtered), len(tt.wantPaths))
			}
		})
	}
}

func TestAssembleDiff(t *testing.T) {
	t.Parallel()
	files := []ClassifiedFile{
		{Diff: "diff a\n"},
		{Diff: "diff b\n"},
	}
	got := AssembleDiff(files)
	if got != "diff a\ndiff b\n" {
		t.Errorf("AssembleDiff = %q", got)
	}
}

func TestAssembleDiff_Empty(t *testing.T) {
	t.Parallel()
	got := AssembleDiff(nil)
	if got != "" {
		t.Errorf("AssembleDiff(nil) = %q, want empty", got)
	}
}

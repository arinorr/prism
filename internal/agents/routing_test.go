package agents

import (
	"testing"
)

func TestClassifyPR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		paths []string
		want  PRCategory
	}{
		// Docs
		{"docs: markdown only", []string{"README.md", "CHANGELOG.md"}, PRCategoryDocs},
		{"docs: docs directory", []string{"docs/guide.md", "docs/api.md"}, PRCategoryDocs},
		{"docs: mixed doc types", []string{"README.md", "docs/guide.rst", "LICENSE"}, PRCategoryDocs},
		{"docs: txt files", []string{"notes.txt", "TODO.txt"}, PRCategoryDocs},
		{"docs: contributing", []string{"CONTRIBUTING.md"}, PRCategoryDocs},
		{"docs: adoc", []string{"docs/arch.adoc"}, PRCategoryDocs},

		// Tests
		{"tests: go test files", []string{"handler_test.go", "service_test.go"}, PRCategoryTests},
		{"tests: ts test files", []string{"handler.test.ts", "service.spec.ts"}, PRCategoryTests},
		{"tests: js test files", []string{"utils.test.js", "app.spec.js"}, PRCategoryTests},
		{"tests: test directory", []string{"test/helper.go", "test/fixtures/data.json"}, PRCategoryTests},
		{"tests: __tests__ directory", []string{"__tests__/app.tsx"}, PRCategoryTests},
		{"tests: tests directory", []string{"tests/integration/api.go"}, PRCategoryTests},
		{"tests: tsx spec", []string{"Component.spec.tsx"}, PRCategoryTests},

		// Config
		{"config: yaml", []string{".github/workflows/ci.yml", "docker-compose.yml"}, PRCategoryConfig},
		{"config: dockerfile", []string{"Dockerfile", "Dockerfile.dev"}, PRCategoryConfig},
		{"config: env files", []string{".env", ".env.local", ".env.production"}, PRCategoryConfig},
		{"config: go mod", []string{"go.mod", "go.sum"}, PRCategoryConfig},
		{"config: tsconfig", []string{"tsconfig.json"}, PRCategoryConfig},
		{"config: eslintrc", []string{".eslintrc.json"}, PRCategoryConfig},
		{"config: gitignore", []string{".gitignore"}, PRCategoryConfig},
		{"config: toml", []string{"config.toml"}, PRCategoryConfig},
		{"config: json", []string{"package.json", "settings.json"}, PRCategoryConfig},
		{"config: github dir", []string{".github/CODEOWNERS", ".github/pull_request_template.md"}, PRCategoryConfig},
		{"config: ci dir", []string{"ci/build.sh"}, PRCategoryConfig},

		// Code (anything else)
		{"code: go files", []string{"main.go", "handler.go"}, PRCategoryCode},
		{"code: ts files", []string{"app.ts", "utils.ts"}, PRCategoryCode},
		{"code: mixed extensions", []string{"server.go", "client.py"}, PRCategoryCode},

		// Mixed (any disagreement → code)
		{"mixed: code + docs", []string{"handler.go", "README.md"}, PRCategoryCode},
		{"mixed: code + tests", []string{"handler.go", "handler_test.go"}, PRCategoryCode},
		{"mixed: code + config", []string{"main.go", "Dockerfile"}, PRCategoryCode},
		{"mixed: docs + config", []string{"README.md", ".gitignore"}, PRCategoryCode},
		{"mixed: tests + config", []string{"handler_test.go", "go.mod"}, PRCategoryCode},

		// Edge cases
		{"empty", []string{}, PRCategoryCode},
		{"single code file", []string{"main.go"}, PRCategoryCode},
		{"single doc file", []string{"README.md"}, PRCategoryDocs},
		{"single test file", []string{"handler_test.go"}, PRCategoryTests},
		{"single config file", []string{"Dockerfile"}, PRCategoryConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyPR(tt.paths)
			if got != tt.want {
				t.Errorf("ClassifyPR(%v) = %q, want %q", tt.paths, got, tt.want)
			}
		})
	}
}

func TestClassifyFile_Precedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want PRCategory
	}{
		// Directory prefix wins over extension.
		{"json in test dir", "test/fixtures/data.json", PRCategoryTests},
		{"yml in test dir", "test/config.yml", PRCategoryTests},
		{"md in test dir", "test/README.md", PRCategoryTests},
		{"json in docs dir", "docs/schema.json", PRCategoryDocs},

		// Basename wins over extension.
		{"README without ext", "README", PRCategoryDocs},
		{"LICENSE without ext", "LICENSE", PRCategoryDocs},
		{"Dockerfile no ext", "Dockerfile", PRCategoryConfig},
		{"Dockerfile.dev", "Dockerfile.dev", PRCategoryConfig},
		{"docker-compose.yml", "docker-compose.yml", PRCategoryConfig},
		{"docker-compose.override.yml", "docker-compose.override.yml", PRCategoryConfig},

		// Test suffixes win over .go extension.
		{"go test file", "handler_test.go", PRCategoryTests},
		{"ts test file", "handler.test.ts", PRCategoryTests},
		{"tsx spec file", "Component.spec.tsx", PRCategoryTests},

		// Extensions.
		{"plain md", "notes.md", PRCategoryDocs},
		{"plain yml", "config.yml", PRCategoryConfig},
		{"plain json", "data.json", PRCategoryConfig},
		{"plain go", "main.go", PRCategoryCode},
		{"plain ts", "app.ts", PRCategoryCode},
		{"plain sh", "deploy.sh", PRCategoryCode},

		// Shell scripts don't match config.
		{"sh file", "scripts/deploy.sh", PRCategoryCode},

		// Nested github dir.
		{"github workflow", ".github/workflows/ci.yml", PRCategoryConfig},
		{"github codeowners", ".github/CODEOWNERS", PRCategoryConfig},
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

func TestRouteAgents(t *testing.T) {
	t.Parallel()

	allRoles := AllRoles

	tests := []struct {
		name     string
		category PRCategory
		wantSlugs []string
	}{
		{
			"code: all agents",
			PRCategoryCode,
			[]string{"know-it-all", "architect", "solver", "editor", "optimizer", "sentinel", "test-engineer"},
		},
		{
			"docs: know-it-all + editor",
			PRCategoryDocs,
			[]string{"know-it-all", "editor"},
		},
		{
			"tests: test-engineer + know-it-all + solver",
			PRCategoryTests,
			[]string{"know-it-all", "solver", "test-engineer"},
		},
		{
			"config: know-it-all + sentinel",
			PRCategoryConfig,
			[]string{"know-it-all", "sentinel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			routed := RouteAgents(allRoles, tt.category)

			gotSlugs := make(map[string]bool)
			for _, r := range routed {
				gotSlugs[r.Slug] = true
			}

			for _, want := range tt.wantSlugs {
				if !gotSlugs[want] {
					t.Errorf("expected role %q in routed set", want)
				}
			}
			if len(routed) != len(tt.wantSlugs) {
				t.Errorf("routed %d roles, want %d", len(routed), len(tt.wantSlugs))
				for _, r := range routed {
					t.Logf("  got: %s", r.Slug)
				}
			}
		})
	}
}

func TestRouteAgents_NilRelevanceAlwaysIncluded(t *testing.T) {
	t.Parallel()

	role := Role{
		Name: "Universal",
		Slug: "universal",
		// Relevance: nil — should be included for all categories.
	}

	for _, cat := range []PRCategory{PRCategoryDocs, PRCategoryTests, PRCategoryConfig, PRCategoryCode} {
		routed := RouteAgents([]Role{role}, cat)
		if len(routed) != 1 {
			t.Errorf("role with nil Relevance excluded for category %q", cat)
		}
	}
}

func TestRouteAgents_EmptyRoles(t *testing.T) {
	t.Parallel()
	routed := RouteAgents(nil, PRCategoryDocs)
	if len(routed) != 0 {
		t.Errorf("expected 0 roles, got %d", len(routed))
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

package agents

import (
	"path/filepath"
	"slices"
	"strings"
)

// PRCategory classifies a pull request by its content type.
type PRCategory string

const (
	PRCategoryDocs   PRCategory = "docs"
	PRCategoryTests  PRCategory = "tests"
	PRCategoryConfig PRCategory = "config"
	PRCategoryCode   PRCategory = "code"
)

// Valid returns true if the category is one of the known values.
func (c PRCategory) Valid() bool {
	switch c {
	case PRCategoryDocs, PRCategoryTests, PRCategoryConfig, PRCategoryCode:
		return true
	}
	return false
}

// ClassifyPR determines the PR category from its file paths.
// Returns PRCategoryCode if the list is empty, or if any file doesn't match
// a specialized category, or if files disagree on category.
func ClassifyPR(paths []string) PRCategory {
	if len(paths) == 0 {
		return PRCategoryCode
	}

	category := classifyFile(paths[0])
	for _, p := range paths[1:] {
		if classifyFile(p) != category {
			return PRCategoryCode
		}
	}
	return category
}

// ClassifyFile returns the category for a single file path.
// Exported for verbose logging in cmd/review.go.
// Checks directory prefix first, then basename, then extension.
func ClassifyFile(path string) PRCategory {
	return classifyFile(path)
}

// --- Directory prefix patterns (checked first) ---

var docsDirPrefixes = []string{"docs/"}

var testsDirPrefixes = []string{"test/", "__tests__/", "tests/"}

var configDirPrefixes = []string{".github/", "ci/"}

// --- Basename patterns (checked second) ---

var docsBasenames = []string{"README", "CHANGELOG", "LICENSE", "CONTRIBUTING"}

var configBasenames = []string{"Dockerfile", "docker-compose"}

// --- Extension patterns (checked third) ---

var docsExtensions = []string{".md", ".txt", ".rst", ".adoc"}

var testsExtensions = []string{} // tests are identified by suffix/directory, not bare extension

var configExtensions = []string{".yml", ".yaml", ".toml", ".ini"}

// testsSuffixes are checked against the full basename (not just extension).
var testsSuffixes = []string{
	"_test.go",
	".test.ts", ".test.js", ".test.tsx", ".test.jsx",
	".spec.ts", ".spec.js", ".spec.tsx", ".spec.jsx",
}

// configExactNames are full basenames that indicate config files.
var configExactNames = []string{
	".gitignore", ".eslintrc", ".eslintrc.js", ".eslintrc.json", ".eslintrc.yml",
	"tsconfig.json", "go.mod", "go.sum",
}

// configEnvPrefix matches .env, .env.local, .env.production, etc.
const configEnvPrefix = ".env"

// configJSONExtension is handled specially: .json is config ONLY when not
// inside a test/docs directory (those take precedence via directory matching).
const configJSONExtension = ".json"

func classifyFile(path string) PRCategory {
	path = filepath.ToSlash(path) // normalize separators
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))

	// --- Pass 1: Directory prefix (highest priority) ---

	for _, prefix := range docsDirPrefixes {
		if strings.HasPrefix(path, prefix) {
			return PRCategoryDocs
		}
	}
	for _, prefix := range testsDirPrefixes {
		if strings.HasPrefix(path, prefix) {
			return PRCategoryTests
		}
	}
	for _, prefix := range configDirPrefixes {
		if strings.HasPrefix(path, prefix) {
			return PRCategoryConfig
		}
	}

	// --- Pass 2: Basename patterns ---

	baseUpper := strings.ToUpper(base)
	baseNoExt := strings.TrimSuffix(base, ext)
	baseNoExtUpper := strings.ToUpper(baseNoExt)

	for _, name := range docsBasenames {
		if baseNoExtUpper == name || baseUpper == name {
			return PRCategoryDocs
		}
	}
	for _, name := range configBasenames {
		if strings.HasPrefix(base, name) {
			return PRCategoryConfig
		}
	}
	for _, exact := range configExactNames {
		if base == exact {
			return PRCategoryConfig
		}
	}
	if strings.HasPrefix(base, configEnvPrefix) {
		return PRCategoryConfig
	}

	// --- Pass 3: Test suffixes (before extension, since _test.go has .go ext) ---

	for _, suffix := range testsSuffixes {
		if strings.HasSuffix(base, suffix) {
			return PRCategoryTests
		}
	}

	// --- Pass 4: Extension ---

	for _, e := range docsExtensions {
		if ext == e {
			return PRCategoryDocs
		}
	}
	for _, e := range configExtensions {
		if ext == e {
			return PRCategoryConfig
		}
	}
	if ext == configJSONExtension {
		return PRCategoryConfig
	}

	return PRCategoryCode
}

// RouteAgents returns the subset of roles appropriate for the PR category.
// Returns all roles unchanged for PRCategoryCode.
func RouteAgents(roles []Role, category PRCategory) []Role {
	if category == PRCategoryCode {
		return roles
	}
	out := make([]Role, 0, len(roles))
	for _, r := range roles {
		if len(r.Relevance) == 0 || slices.Contains(r.Relevance, category) {
			out = append(out, r)
		}
	}
	return out
}

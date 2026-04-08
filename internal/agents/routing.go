package agents

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/arinorr/prism/internal/diff"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/index"
	"github.com/arinorr/prism/internal/resolve"
)

// PRCategory classifies a pull request file by its content type.
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

// ClassifiedFile bundles a file's path, category, and diff section.
type ClassifiedFile struct {
	Path     string
	Category PRCategory
	Diff     string
}

// ReviewContext holds per-review data built at the start of Review()
// and passed explicitly to each agent dispatch.
type ReviewContext struct {
	Files     []ClassifiedFile
	Index     *index.Index       // nil if index build failed
	Resolver  *resolve.Resolver  // nil if index build failed
	ChangeMap *ChangeMap         // nil if index build failed
}

// SymbolStatus returns the change status of a symbol, nil-safe.
// Returns SymbolExisting when ChangeMap is nil (graceful degradation).
func (rc *ReviewContext) SymbolStatus(file, name string) SymbolStatus {
	if rc == nil || rc.ChangeMap == nil {
		return SymbolExisting
	}
	return rc.ChangeMap.Status(file, name)
}

// AgentPromptContext groups prompt-specific pieces to avoid
// multiple undifferentiated string parameters.
type AgentPromptContext struct {
	Diff       string
	CrossRefs  string
	ScopeHints string
}

// --- Classification ---

// classifier is a function that attempts to classify a file.
// Returns the category and true if matched, or false to try the next classifier.
type classifier func(path, base, ext string) (PRCategory, bool)

var classifiers = []classifier{
	classifyByDirectory,
	classifyByBasename,
	classifyBySuffix,
	classifyByExtension,
}

// classifyFile returns the category for a single file path.
func classifyFile(path string) PRCategory {
	path = filepath.ToSlash(path)
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(path))
	for _, c := range classifiers {
		if cat, ok := c(path, base, ext); ok {
			return cat
		}
	}
	return PRCategoryCode
}

// ClassifyFile is the exported version for verbose logging.
func ClassifyFile(path string) PRCategory { return classifyFile(path) }

// Pass 1: Directory prefix.
var docsDirPrefixes = []string{"docs/"}
var testsDirPrefixes = []string{"test/", "__tests__/", "tests/"}
var configDirPrefixes = []string{".github/", "ci/"}

func classifyByDirectory(path, _, _ string) (PRCategory, bool) {
	for _, p := range docsDirPrefixes {
		if strings.HasPrefix(path, p) {
			return PRCategoryDocs, true
		}
	}
	for _, p := range testsDirPrefixes {
		if strings.HasPrefix(path, p) {
			return PRCategoryTests, true
		}
	}
	for _, p := range configDirPrefixes {
		if strings.HasPrefix(path, p) {
			return PRCategoryConfig, true
		}
	}
	return "", false
}

// Pass 2: Basename.
var docsBasenames = []string{"README", "CHANGELOG", "LICENSE", "CONTRIBUTING"}
var configBasePrefixes = []string{"Dockerfile", "docker-compose"}
var configExactNames = map[string]bool{
	".gitignore": true, ".eslintrc": true, ".eslintrc.js": true,
	".eslintrc.json": true, ".eslintrc.yml": true,
	"tsconfig.json": true, "go.mod": true, "go.sum": true,
}

func classifyByBasename(_, base, ext string) (PRCategory, bool) {
	baseNoExt := strings.TrimSuffix(base, ext)
	upper := strings.ToUpper(baseNoExt)
	for _, name := range docsBasenames {
		if upper == name {
			return PRCategoryDocs, true
		}
	}
	for _, prefix := range configBasePrefixes {
		if strings.HasPrefix(base, prefix) {
			return PRCategoryConfig, true
		}
	}
	if configExactNames[base] {
		return PRCategoryConfig, true
	}
	if strings.HasPrefix(base, ".env") {
		return PRCategoryConfig, true
	}
	return "", false
}

// Pass 3: Suffix convention (before extension — _test.go has .go ext).
var testSuffixes = []string{
	"_test.go",
	".test.ts", ".test.js", ".test.tsx", ".test.jsx",
	".spec.ts", ".spec.js", ".spec.tsx", ".spec.jsx",
}

func classifyBySuffix(_, base, _ string) (PRCategory, bool) {
	for _, suffix := range testSuffixes {
		if strings.HasSuffix(base, suffix) {
			return PRCategoryTests, true
		}
	}
	return "", false
}

// Pass 4: Extension.
var docsExtensions = map[string]bool{".md": true, ".txt": true, ".rst": true, ".adoc": true}
var configExtensions = map[string]bool{".yml": true, ".yaml": true, ".toml": true, ".ini": true, ".json": true}

func classifyByExtension(_, _, ext string) (PRCategory, bool) {
	if docsExtensions[ext] {
		return PRCategoryDocs, true
	}
	if configExtensions[ext] {
		return PRCategoryConfig, true
	}
	return "", false
}

// --- Filtering and Assembly ---

// FilterFilesForRole returns the ClassifiedFiles relevant to a role.
func FilterFilesForRole(role *Role, files []ClassifiedFile) []ClassifiedFile {
	if role.SeeAll {
		return files
	}
	out := make([]ClassifiedFile, 0, len(files))
	for _, f := range files {
		if slices.Contains(role.Relevance, f.Category) {
			out = append(out, f)
		}
	}
	return out
}

// AssembleDiff joins the diff sections from classified files into one string.
func AssembleDiff(files []ClassifiedFile) string {
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f.Diff)
	}
	return b.String()
}

// BuildReviewContext classifies files, splits the diff, and optionally
// builds the change map if an index is available.
func BuildReviewContext(pr *gh.PR, idx *index.Index, resolver *resolve.Resolver) *ReviewContext {
	fileDiffs := diff.SplitToMap(pr.Diff)

	var files []ClassifiedFile
	for path, d := range fileDiffs {
		files = append(files, ClassifiedFile{
			Path:     path,
			Category: classifyFile(path),
			Diff:     d,
		})
	}

	// Also classify files from pr.Files that may not be in the diff
	// (e.g., deleted files with no diff content).
	inDiff := make(map[string]bool, len(files))
	for _, f := range files {
		inDiff[f.Path] = true
	}
	for _, f := range pr.Files {
		if !inDiff[f.Path] {
			files = append(files, ClassifiedFile{
				Path:     f.Path,
				Category: classifyFile(f.Path),
			})
		}
	}

	// Sort files alphabetically for deterministic output and cache-friendly
	// prefix sharing across agents. Agents that share a subset of files will
	// have byte-identical diff content at the same positions in their prompts.
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	rc := &ReviewContext{
		Files:    files,
		Index:    idx,
		Resolver: resolver,
	}

	if idx != nil {
		rc.ChangeMap = BuildChangeMap(files, idx)
	}

	return rc
}

// RoutingSummary returns a human-readable summary of how files were routed.
func RoutingSummary(files []ClassifiedFile, roles []Role) string {
	counts := make(map[PRCategory]int)
	for _, f := range files {
		counts[f.Category]++
	}

	var b strings.Builder
	for _, cat := range []PRCategory{PRCategoryCode, PRCategoryDocs, PRCategoryTests, PRCategoryConfig} {
		n := counts[cat]
		if n == 0 {
			continue
		}
		var names []string
		for _, r := range roles {
			if r.SeeAll || slices.Contains(r.Relevance, cat) {
				names = append(names, r.Name)
			}
		}
		fmt.Fprintf(&b, "   %d %s file(s) → %s\n", n, cat, strings.Join(names, ", "))
	}
	return b.String()
}

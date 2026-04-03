package agents

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/arinorr/prism/internal/gh"
)

// extToLanguage maps file extensions to language identifiers.
var extToLanguage = map[string]string{
	".ts":  "typescript",
	".tsx": "typescript",
	".js":  "typescript",
	".jsx": "typescript",
	".mjs": "typescript",
	".cjs": "typescript",
	".go":  "go",
}

// DetectLanguages returns a deduplicated, sorted list of language identifiers
// present in the given file changes. Unknown extensions are ignored.
func DetectLanguages(files []gh.FileChange) []string {
	seen := make(map[string]bool)
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if lang, ok := extToLanguage[ext]; ok {
			seen[lang] = true
		}
	}

	languages := make([]string, 0, len(seen))
	for lang := range seen {
		languages = append(languages, lang)
	}
	slices.Sort(languages)
	return languages
}

// languageSkillPath derives the language module path from a base skill path.
// Example: "skills/know-it-all.md" + "typescript" yields "skills/know-it-all/typescript.md".
func languageSkillPath(baseSkillFile, language string) string {
	ext := filepath.Ext(baseSkillFile)
	dir := strings.TrimSuffix(baseSkillFile, ext)
	return filepath.Join(dir, language+ext)
}

package parse

// extToLanguage maps file extensions to language identifiers.
// This is intentionally duplicated from internal/agents/languages.go
// to avoid an import cycle. A test asserts both maps stay in sync.
var extToLanguage = map[string]string{
	".ts":  "typescript",
	".tsx": "typescript",
	".js":  "typescript",
	".jsx": "typescript",
	".mjs": "typescript",
	".cjs": "typescript",
	".go":  "go",
}

// ExtToLanguage returns a copy of the extension-to-language map.
// Exported for use in tests that verify consistency with the agents package.
func ExtToLanguage() map[string]string {
	cp := make(map[string]string, len(extToLanguage))
	for k, v := range extToLanguage {
		cp[k] = v
	}
	return cp
}

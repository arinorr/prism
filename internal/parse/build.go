package parse

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skipDirs contains directory names that should be skipped during indexing.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	"testdata":     true,
	"__pycache__":  true,
}

// maxFileSize is the maximum file size to index (1 MB). Larger files are
// likely generated and would slow down scanning without adding value.
const maxFileSize = 1 << 20

// Build walks rootDir, scans source files for the given languages, and
// returns a frozen Index. Only files whose extension maps to one of the
// requested languages are scanned. The context can be used to cancel a
// long-running walk.
func Build(ctx context.Context, rootDir string, languages []string) (*Index, error) {
	langSet := make(map[string]bool, len(languages))
	for _, l := range languages {
		langSet[l] = true
	}

	scanners := map[string]LanguageScanner{
		"go":         GoScanner{},
		"typescript": NewTSScanner(),
	}

	idx := NewIndex()

	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		// Check for cancellation periodically.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := extToLanguage[ext]
		if !ok || !langSet[lang] {
			return nil
		}

		scanner, ok := scanners[lang]
		if !ok {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}

		src, err := os.ReadFile(path) // #nosec G304 G122 -- path comes from WalkDir on a user-owned repo; symlink traversal is acceptable here
		if err != nil {
			return nil
		}

		// Use relative path from rootDir for consistent symbol file paths.
		relPath, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			relPath = path
		}

		symbols := scanner.Scan(relPath, src)
		for _, sym := range symbols {
			idx.Add(sym)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", rootDir, err)
	}

	idx.Freeze()
	return idx, nil
}

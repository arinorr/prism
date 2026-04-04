// Package diff provides utilities for preprocessing unified diffs
// to reduce token consumption before sending to LLM agents.
package diff

import (
	"path/filepath"
	"strings"
)

// Lock file patterns — these never contain reviewable code.
var lockFilePatterns = []string{
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"go.sum",
	"Gemfile.lock",
	"Pipfile.lock",
	"Cargo.lock",
	"poetry.lock",
	"composer.lock",
	"packages.lock.json",
	"flake.lock",
}

// Generated code directory/file patterns.
var generatedPatterns = []string{
	"*.pb.go",
	"*.pb.ts",
	"*.pb.js",
	"*_generated.go",
	"*_gen.go",
	"generated/*",
	"__generated__/*",
	"dist/*",
	"build/*",
	".next/*",
}

// Vendor directory patterns.
var vendorPatterns = []string{
	"vendor/*",
	"node_modules/*",
}

// isLockFile returns true if the file path matches a known lock file.
func isLockFile(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range lockFilePatterns {
		if base == pattern {
			return true
		}
	}
	return false
}

// isGenerated returns true if the path matches a generated code pattern.
func isGenerated(path string) bool {
	return matchesAny(path, generatedPatterns)
}

// isVendor returns true if the path is inside a vendor directory.
func isVendor(path string) bool {
	return matchesAny(path, vendorPatterns)
}

// matchesAny checks if the path matches any of the glob patterns.
// Handles directory prefix patterns like "vendor/*" by checking if the
// path starts with the directory prefix.
func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Try matching the full path.
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		// For directory patterns (ending in /*), check prefix match.
		// e.g. "vendor/*" should match "vendor/lib/util.go".
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "/*")
			parts := strings.Split(path, "/")
			for i := range parts {
				if parts[i] == prefix {
					return true
				}
			}
		}
		// Try matching just the filename against non-directory patterns.
		base := filepath.Base(path)
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// matchesCustom checks if the path matches any custom glob patterns.
func matchesCustom(path string, patterns []string) bool {
	return matchesAny(path, patterns)
}

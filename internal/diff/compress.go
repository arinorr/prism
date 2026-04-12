package diff

import (
	"fmt"
	"strings"
)

// CompressOptions controls what gets stripped from a diff.
type CompressOptions struct {
	// StripLockFiles removes lock file changes (default: true).
	StripLockFiles bool
	// StripGenerated removes generated/compiled code (default: true).
	StripGenerated bool
	// StripBinary removes binary file diffs (default: true).
	StripBinary bool
	// StripVendor removes vendor/node_modules changes (default: true).
	StripVendor bool
	// ContextLines controls how many context lines to keep per hunk.
	// -1 = keep all (no stripping), 0 = strip all, 1-3 = keep N lines.
	// Default: 1.
	ContextLines int
	// ExtraPatterns allows custom glob patterns to strip.
	ExtraPatterns []string
}

// DefaultOptions returns sensible defaults for diff compression.
func DefaultOptions() CompressOptions {
	return CompressOptions{
		StripLockFiles: true,
		StripGenerated: true,
		StripBinary:    true,
		StripVendor:    true,
		ContextLines:   1,
	}
}

// NoCompression returns options that disable all compression.
func NoCompression() CompressOptions {
	return CompressOptions{
		ContextLines: -1,
	}
}

// Summary describes what was removed during compression.
type Summary struct {
	OriginalBytes   int
	CompressedBytes int
	FilesRemoved    []string
	ContextReduced  bool
}

// Compress preprocesses a unified diff, removing low-value content.
// Returns the compressed diff and a summary of what was removed.
func Compress(rawDiff string, opts CompressOptions) (string, Summary) {
	summary := Summary{
		OriginalBytes: len(rawDiff),
	}

	if rawDiff == "" {
		return "", summary
	}

	// Pass 1: Split into per-file sections and filter.
	sections := splitDiffByFile(rawDiff)
	var kept []string

	for _, section := range sections {
		path := extractFilePath(section)

		if opts.StripBinary && isBinaryDiff(section) {
			summary.FilesRemoved = append(summary.FilesRemoved, path)
			continue
		}

		if opts.StripLockFiles && isLockFile(path) {
			summary.FilesRemoved = append(summary.FilesRemoved, path)
			continue
		}

		if opts.StripGenerated && isGenerated(path) {
			summary.FilesRemoved = append(summary.FilesRemoved, path)
			continue
		}

		if opts.StripVendor && isVendor(path) {
			summary.FilesRemoved = append(summary.FilesRemoved, path)
			continue
		}

		if len(opts.ExtraPatterns) > 0 && matchesCustom(path, opts.ExtraPatterns) {
			summary.FilesRemoved = append(summary.FilesRemoved, path)
			continue
		}

		// Pass 2: Context reduction.
		if opts.ContextLines >= 0 {
			section = reduceContext(section, opts.ContextLines)
			summary.ContextReduced = true
		}

		kept = append(kept, section)
	}

	result := strings.Join(kept, "")
	summary.CompressedBytes = len(result)
	return result, summary
}

// splitDiffByFile splits a unified diff into per-file sections.
// Each section starts with "diff --git " and includes all content
// until the next "diff --git " or end of string.
func splitDiffByFile(rawDiff string) []string {
	const marker = "diff --git "
	var sections []string

	lines := strings.Split(rawDiff, "\n")
	var current []string

	for _, line := range lines {
		if strings.HasPrefix(line, marker) && len(current) > 0 {
			sections = append(sections, strings.Join(current, "\n")+"\n")
			current = nil
		}
		current = append(current, line)
	}

	if len(current) > 0 {
		text := strings.Join(current, "\n")
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		sections = append(sections, text)
	}

	return sections
}

// extractFilePath pulls the file path from a diff section header.
// Handles "diff --git a/path b/path" format.
func extractFilePath(section string) string {
	firstLine := section
	if idx := strings.Index(section, "\n"); idx != -1 {
		firstLine = section[:idx]
	}

	// "diff --git a/foo/bar.go b/foo/bar.go"
	if strings.HasPrefix(firstLine, "diff --git ") {
		parts := strings.Fields(firstLine)
		if len(parts) >= 4 {
			// Use the b/ path (destination).
			path := parts[3]
			path = strings.TrimPrefix(path, "b/")
			return path
		}
	}

	return ""
}

// SplitToMap splits a unified diff into per-file sections keyed by file path.
// Sections whose path cannot be extracted are logged and skipped.
func SplitToMap(rawDiff string) map[string]string {
	sections := splitDiffByFile(rawDiff)
	m := make(map[string]string, len(sections))
	for _, section := range sections {
		path := extractFilePath(section)
		if path == "" {
			continue
		}
		m[path] = section
	}
	return m
}

// isBinaryDiff returns true if the section is a binary file diff.
func isBinaryDiff(section string) bool {
	return strings.Contains(section, "Binary files ") && strings.Contains(section, " differ")
}

// reduceContext keeps only N context lines before the first change and
// after the last change in each hunk.
func reduceContext(section string, maxContext int) string {
	lines := strings.Split(strings.TrimRight(section, "\n"), "\n")
	var result []string
	inHunk := false
	var hunkLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// Flush previous hunk.
			if inHunk && len(hunkLines) > 0 {
				result = append(result, compressHunk(hunkLines, maxContext)...)
			}
			inHunk = true
			hunkLines = []string{line}
			continue
		}

		if inHunk {
			hunkLines = append(hunkLines, line)
		} else {
			// File header lines before the first hunk.
			result = append(result, line)
		}
	}

	// Flush last hunk.
	if inHunk && len(hunkLines) > 0 {
		result = append(result, compressHunk(hunkLines, maxContext)...)
	}

	return strings.Join(result, "\n") + "\n"
}

// compressHunk reduces context lines in a single hunk.
// hunkLines[0] is the @@ marker, rest are content lines.
func compressHunk(hunkLines []string, maxContext int) []string {
	if len(hunkLines) < 2 {
		return hunkLines
	}

	header := hunkLines[0]
	content := hunkLines[1:]

	// Find ranges of change lines (+/-) and keep only maxContext
	// context lines before/after each range.
	type lineInfo struct {
		text     string
		isChange bool
	}

	var infos []lineInfo
	for _, line := range content {
		isChange := strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")
		// Don't count file headers as changes.
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			isChange = false
		}
		infos = append(infos, lineInfo{text: line, isChange: isChange})
	}

	// Mark which context lines to keep (within maxContext of a change).
	keep := make([]bool, len(infos))
	for i, info := range infos {
		if info.isChange {
			keep[i] = true
			// Keep N lines before.
			for j := 1; j <= maxContext && i-j >= 0; j++ {
				keep[i-j] = true
			}
			// Keep N lines after.
			for j := 1; j <= maxContext && i+j < len(infos); j++ {
				keep[i+j] = true
			}
		}
	}

	// Build compressed content.
	var compressed []string
	skipped := 0
	for i, info := range infos {
		if keep[i] {
			if skipped > 0 {
				compressed = append(compressed, fmt.Sprintf(" ... (%d lines omitted)", skipped))
				skipped = 0
			}
			compressed = append(compressed, info.text)
		} else {
			skipped++
		}
	}
	if skipped > 0 {
		compressed = append(compressed, fmt.Sprintf(" ... (%d lines omitted)", skipped))
	}

	// Rebuild with simplified header (don't recount — the omission
	// markers make exact line counts misleading anyway).
	return append([]string{header}, compressed...)
}

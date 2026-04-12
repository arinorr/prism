package agents

import (
	"fmt"
	"strings"

	"github.com/arinorr/prism/internal/parse"
	"github.com/arinorr/prism/internal/difflex"
)

// SymbolStatus indicates whether a symbol is new, modified, or pre-existing.
type SymbolStatus string

const (
	SymbolAdded    SymbolStatus = "new in this PR"
	SymbolModified SymbolStatus = "modified in this PR"
	SymbolExisting SymbolStatus = "pre-existing"
)

// SymbolKey uniquely identifies a symbol by file and name.
// Avoids collisions when the same name exists in multiple files.
type SymbolKey struct {
	File string
	Name string
}

func (k SymbolKey) String() string {
	return fmt.Sprintf("%s (%s)", k.Name, k.File)
}

// ChangeMap tracks which symbols were introduced, modified, or pre-existed.
type ChangeMap struct {
	Modified map[SymbolKey]bool
	Added    map[SymbolKey]bool
}

// Status returns the change status of a symbol.
func (cm *ChangeMap) Status(file, name string) SymbolStatus {
	key := SymbolKey{File: file, Name: name}
	if cm.Added[key] {
		return SymbolAdded
	}
	if cm.Modified[key] {
		return SymbolModified
	}
	return SymbolExisting
}

// BuildChangeMap cross-references diff changed-line ranges against the index.
//
// Algorithm:
//  1. For + lines with declaration patterns, mark as added.
//  2. For indexed symbols whose line range overlaps changed lines, mark as modified.
//  3. Added symbols are skipped in step 2.
func BuildChangeMap(files []ClassifiedFile, idx *parse.Index) *ChangeMap {
	cm := &ChangeMap{
		Modified: make(map[SymbolKey]bool),
		Added:    make(map[SymbolKey]bool),
	}

	for _, f := range files {
		if f.Diff == "" {
			continue
		}

		changedLines := parseChangedLines(f.Diff)
		if len(changedLines) == 0 {
			continue
		}

		// Step 1: Find declarations on + lines → added.
		candidates := difflex.ExtractCandidates(f.Diff)
		for _, c := range candidates {
			if c.Kind == difflex.RefDeclaration && c.InChange {
				cm.Added[SymbolKey{File: f.Path, Name: c.Name}] = true
			}
		}

		// Step 2: Find indexed symbols whose range overlaps changed lines → modified.
		syms := idx.SymbolsInFile(f.Path)
		for _, sym := range syms {
			key := SymbolKey{File: f.Path, Name: sym.Name}
			if cm.Added[key] {
				continue // already marked as added, skip
			}
			if rangeOverlapsChanges(sym.StartLine, sym.EndLine, changedLines) {
				cm.Modified[key] = true
			}
		}
	}

	return cm
}

// FormatScopeHints formats the change map as scope context for agent prompts.
func (cm *ChangeMap) FormatScopeHints() string {
	if cm == nil || (len(cm.Added) == 0 && len(cm.Modified) == 0) {
		return ""
	}

	var b strings.Builder

	if len(cm.Modified) > 0 {
		b.WriteString("Symbols modified in this PR: ")
		first := true
		for key := range cm.Modified {
			if !first {
				b.WriteString(", ")
			}
			b.WriteString(key.String())
			first = false
		}
		b.WriteString("\n")
	}

	if len(cm.Added) > 0 {
		b.WriteString("Symbols new in this PR: ")
		first := true
		for key := range cm.Added {
			if !first {
				b.WriteString(", ")
			}
			b.WriteString(key.String())
			first = false
		}
		b.WriteString("\n")
	}

	b.WriteString("All other symbols in the codebase are pre-existing.")

	return b.String()
}

// parseChangedLines extracts line numbers that have +/- prefixes from a diff.
// Returns a sorted set of line numbers (1-based, from the new file side for +).
func parseChangedLines(diffText string) map[int]bool {
	changed := make(map[int]bool)
	newLine := 0
	inHunk := false

	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "@@") {
			// Parse hunk header: @@ -old,count +new,count @@
			newLine = parseHunkNewStart(line)
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}

		if line == "" {
			newLine++
			continue
		}

		switch line[0] {
		case '+':
			changed[newLine] = true
			newLine++
		case '-':
			changed[newLine] = true
			// Don't increment newLine for removed lines.
		case ' ':
			newLine++
		}
	}

	return changed
}

// parseHunkNewStart extracts the new-file start line from a hunk header.
// e.g., "@@ -10,5 +15,7 @@" returns 15.
func parseHunkNewStart(hunkHeader string) int {
	// Find +N in the header.
	idx := strings.Index(hunkHeader, "+")
	if idx < 0 {
		return 0
	}
	rest := hunkHeader[idx+1:]
	var n int
	_, _ = fmt.Sscanf(rest, "%d", &n)
	return n
}

// rangeOverlapsChanges returns true if [start, end] contains any changed line.
func rangeOverlapsChanges(start, end int, changed map[int]bool) bool {
	for line := start; line <= end; line++ {
		if changed[line] {
			return true
		}
	}
	return false
}

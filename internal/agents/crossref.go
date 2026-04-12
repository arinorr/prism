package agents

import (
	"fmt"
	"sort"
	"strings"

	"github.com/arinorr/prism/internal/index"
	"github.com/arinorr/prism/internal/lex"
)

// Cross-reference caps.
const (
	maxCrossRefs     = 5
	maxCrossRefLines = 30
)

// CrossReference is a resolved symbol for context injection.
type CrossReference struct {
	Symbol         index.Symbol
	Text           string // definition source text
	ReferencedFrom string // which diff file referenced this symbol
}

// FilterByIndex keeps only candidates whose names exist in the index.
func FilterByIndex(candidates []lex.SymbolReference, idx *index.Index) []lex.SymbolReference {
	var filtered []lex.SymbolReference
	for _, c := range candidates {
		if syms := idx.Lookup(c.Name); len(syms) > 0 {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

// ResolveCrossReferences resolves symbol references from non-code files
// in the agent's diff. Returns structured data — the caller formats.
//
// Only resolves references from non-code files (tests, config) to code files.
// Code-only agents and SeeAll agents don't need cross-references.
func ResolveCrossReferences(
	agentFiles []ClassifiedFile,
	rctx *ReviewContext,
) []CrossReference {
	if rctx == nil || rctx.Index == nil || rctx.Resolver == nil {
		return nil
	}

	// Collect non-code files in the agent's set.
	var nonCodeFiles []ClassifiedFile
	for _, f := range agentFiles {
		if f.Category != PRCategoryCode && f.Diff != "" {
			nonCodeFiles = append(nonCodeFiles, f)
		}
	}
	if len(nonCodeFiles) == 0 {
		return nil
	}

	// Preload files for resolution.
	var filesToPreload []string
	for _, f := range nonCodeFiles {
		filesToPreload = append(filesToPreload, f.Path)
	}
	rctx.Resolver.PreloadFiles(filesToPreload)

	// Extract and filter candidates from non-code diffs.
	type rankedCandidate struct {
		ref  lex.SymbolReference
		file string // which non-code file referenced it
	}
	var candidates []rankedCandidate
	seen := make(map[string]bool)

	for _, f := range nonCodeFiles {
		raw := lex.ExtractCandidates(f.Diff)
		filtered := FilterByIndex(raw, rctx.Index)
		for _, c := range filtered {
			if c.Kind == lex.RefDeclaration {
				continue // declarations aren't references to other code
			}
			if seen[c.Name] {
				// Dedup: keep the one from a changed line if available.
				if c.InChange {
					// Replace — prefer changed-line reference.
					for i := range candidates {
						if candidates[i].ref.Name == c.Name {
							candidates[i] = rankedCandidate{ref: c, file: f.Path}
							break
						}
					}
				}
				continue
			}
			seen[c.Name] = true
			candidates = append(candidates, rankedCandidate{ref: c, file: f.Path})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Rank: changed-line refs first, then calls over types.
	sort.SliceStable(candidates, func(i, j int) bool {
		ci, cj := candidates[i].ref, candidates[j].ref
		if ci.InChange != cj.InChange {
			return ci.InChange // changed first
		}
		if ci.Kind != cj.Kind {
			return ci.Kind == lex.RefCall // calls before types
		}
		return false
	})

	// Cap.
	if len(candidates) > maxCrossRefs {
		candidates = candidates[:maxCrossRefs]
	}

	// Resolve each candidate to its definition text.
	// Preload the definition files.
	var defFiles []string
	for _, c := range candidates {
		syms := rctx.Index.Lookup(c.ref.Name)
		if len(syms) > 0 {
			defFiles = append(defFiles, syms[0].File)
		}
	}
	rctx.Resolver.PreloadFiles(defFiles)

	var refs []CrossReference
	for _, c := range candidates {
		syms := rctx.Index.Lookup(c.ref.Name)
		if len(syms) == 0 {
			continue
		}
		sym := syms[0]

		rc, err := rctx.Resolver.Resolve(sym.File, sym.StartLine)
		if err != nil || rc.EnclosingScope == nil {
			continue
		}

		// Cap the text length.
		text := rc.EnclosingScope.Text
		lines := strings.Split(text, "\n")
		if len(lines) > maxCrossRefLines {
			lines = lines[:maxCrossRefLines]
			lines = append(lines, "    // ... (truncated)")
		}

		refs = append(refs, CrossReference{
			Symbol:         sym,
			Text:           strings.Join(lines, "\n"),
			ReferencedFrom: c.file,
		})
	}

	return refs
}

// FormatCrossReferences formats resolved references for the agent prompt.
func FormatCrossReferences(refs []CrossReference) string {
	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<cross-references>\n")

	for _, ref := range refs {
		fmt.Fprintf(&b, "Referenced from %s:\n", ref.ReferencedFrom)
		fmt.Fprintf(&b, "  %s:%s() (%s, lines %d-%d):\n",
			ref.Symbol.File, ref.Symbol.Name, ref.Symbol.Kind,
			ref.Symbol.StartLine, ref.Symbol.EndLine)
		for _, line := range strings.Split(ref.Text, "\n") {
			fmt.Fprintf(&b, "    %s\n", line)
		}
		b.WriteString("\n")
	}

	b.WriteString("</cross-references>")
	return b.String()
}

// Package resolve extracts code context for findings using a symbol index.
// Given a finding's file and line number, the resolver identifies the enclosing
// scope (function/class/method) and the definitions of symbols referenced within
// that scope. All file I/O happens in PreloadFiles; after that, Resolve is pure
// lookup against an immutable cache.
package resolve

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/arinorr/prism/internal/index"
)

// Caps to prevent context blowup.
const (
	maxReferences    = 10 // max referenced definitions per finding
	maxDefLines      = 50 // max lines per referenced definition
	maxScopeLines    = 150 // max lines for an enclosing scope
)

// ResolvedContext holds the code context needed to verify a finding.
type ResolvedContext struct {
	File           string
	FindingLine    int
	EnclosingScope *ScopeContext      // function/class containing the finding
	References     []ReferenceContext // definitions of symbols used in the scope
}

// ScopeContext is the source text of the enclosing scope.
type ScopeContext struct {
	Symbol index.Symbol
	Text   string
}

// ReferenceContext is the source text of a referenced symbol definition.
type ReferenceContext struct {
	Symbol index.Symbol
	Text   string
}

// Resolver resolves code context for findings using a pre-built symbol index.
// Call PreloadFiles before Resolve to populate the file cache.
// Thread-safe: PreloadFiles and Resolve may be called from multiple goroutines.
type Resolver struct {
	idx       *index.Index
	repoRoot  string
	mu        sync.RWMutex
	fileCache map[string][]byte
}

// NewResolver creates a Resolver backed by the given index.
func NewResolver(idx *index.Index, repoRoot string) *Resolver {
	return &Resolver{
		idx:       idx,
		repoRoot:  repoRoot,
		fileCache: make(map[string][]byte),
	}
}

// PreloadFiles reads the given files into the cache. Paths are relative to
// repoRoot. Files that cannot be read are silently skipped (logged).
// This method should be called once; after it returns the cache is treated
// as immutable.
func (r *Resolver) PreloadFiles(files []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, f := range files {
		if _, ok := r.fileCache[f]; ok {
			continue
		}
		absPath := filepath.Join(r.repoRoot, f)
		data, err := os.ReadFile(absPath) // #nosec G304 -- paths from index, not user input
		if err != nil {
			log.Printf("resolve: skipping %s: %v", f, err)
			continue
		}
		r.fileCache[f] = data
	}
}

// keywords that should not be looked up as symbol references.
var keywords = map[string]bool{
	"if": true, "else": true, "for": true, "while": true, "return": true,
	"switch": true, "case": true, "break": true, "continue": true, "default": true,
	"func": true, "function": true, "var": true, "let": true, "const": true,
	"type": true, "struct": true, "interface": true, "class": true, "import": true,
	"export": true, "package": true, "defer": true, "go": true, "chan": true,
	"map": true, "range": true, "select": true, "nil": true, "null": true,
	"undefined": true, "true": true, "false": true, "new": true, "this": true,
	"self": true, "async": true, "await": true, "try": true, "catch": true,
	"throw": true, "throws": true, "finally": true, "void": true, "string": true,
	"number": true, "boolean": true, "int": true, "float64": true, "error": true,
	"any": true, "unknown": true, "never": true, "readonly": true, "public": true,
	"private": true, "protected": true, "static": true, "abstract": true,
	"extends": true, "implements": true, "super": true, "yield": true,
	"from": true, "as": true, "of": true, "in": true, "is": true,
	"typeof": true, "instanceof": true, "delete": true, "with": true,
	"enum": true, "namespace": true, "module": true, "declare": true,
	"byte": true, "rune": true, "bool": true, "uint": true, "int64": true,
	"fmt": true, "log": true, "os": true, "io": true, "context": true,
	"println": true, "printf": true, "sprintf": true, "errorf": true,
	"make": true, "append": true, "len": true, "cap": true, "close": true,
	"copy": true, "panic": true, "recover": true, "print": true,
	"console": true, "Promise": true, "Error": true, "Array": true,
	"Object": true, "String": true, "Number": true, "Boolean": true,
	"Map": true, "Set": true, "Date": true, "RegExp": true, "JSON": true,
	"Math": true, "Buffer": true, "require": true,
}

var identRe = regexp.MustCompile(`\b([A-Za-z_]\w+)\b`)

// Resolve returns the code context for a finding at the given file and line.
// The file must have been preloaded via PreloadFiles. If the file is not in
// the cache, an error is returned. References whose files are not in the cache
// are silently skipped (less context, not a failure).
func (r *Resolver) Resolve(file string, line int) (*ResolvedContext, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	src, ok := r.fileCache[file]
	if !ok {
		return nil, fmt.Errorf("file %s not preloaded", file)
	}

	rc := &ResolvedContext{
		File:        file,
		FindingLine: line,
	}

	// Find the enclosing scope.
	sym := r.idx.EnclosingScope(file, line)
	if sym == nil {
		return rc, nil // no enclosing scope found; return partial context
	}

	scopeText := extractLines(src, sym.StartLine, sym.EndLine, maxScopeLines)
	rc.EnclosingScope = &ScopeContext{
		Symbol: *sym,
		Text:   scopeText,
	}

	// Scan for referenced symbols.
	matches := identRe.FindAllString(scopeText, -1)
	seen := make(map[string]bool)
	seen[sym.Name] = true // skip self

	for _, name := range matches {
		if len(rc.References) >= maxReferences {
			break
		}
		if seen[name] || keywords[name] || len(name) < 2 {
			continue
		}
		seen[name] = true

		refs := r.idx.Lookup(name)
		if len(refs) == 0 {
			continue
		}

		// Take the first match (typically the definition).
		ref := refs[0]
		if ref.File == file && ref.StartLine >= sym.StartLine && ref.EndLine <= sym.EndLine {
			// Skip if the reference is inside the same scope (e.g., local variable).
			if len(refs) > 1 {
				ref = refs[1]
			} else {
				continue
			}
		}

		refSrc, ok := r.fileCache[ref.File]
		if !ok {
			log.Printf("resolve: reference %s in %s not in cache, skipping", name, ref.File)
			continue
		}

		refText := extractLines(refSrc, ref.StartLine, ref.EndLine, maxDefLines)
		rc.References = append(rc.References, ReferenceContext{
			Symbol: ref,
			Text:   refText,
		})
	}

	return rc, nil
}

// CollectReferenceFiles returns the set of files that would be needed to
// resolve references for the given finding files. This enables two-pass
// preloading: first preload finding files, then call this to discover
// reference files, then preload those too.
func (r *Resolver) CollectReferenceFiles(findingFiles []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fileSet := make(map[string]bool)

	for _, file := range findingFiles {
		src, ok := r.fileCache[file]
		if !ok {
			continue
		}

		// Get all symbols in this file to find scopes.
		syms := r.idx.SymbolsInFile(file)
		for _, sym := range syms {
			scopeText := extractLines(src, sym.StartLine, sym.EndLine, maxScopeLines)
			matches := identRe.FindAllString(scopeText, -1)
			for _, name := range matches {
				if keywords[name] || len(name) < 2 {
					continue
				}
				refs := r.idx.Lookup(name)
				for _, ref := range refs {
					if ref.File != file {
						fileSet[ref.File] = true
					}
				}
			}
		}
	}

	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}
	return files
}

// extractLines returns lines [start, end] from src, capped at maxLines.
// Line numbers are 1-based.
func extractLines(src []byte, start, end, maxLines int) string {
	lines := strings.Split(string(src), "\n")

	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if end-start+1 > maxLines {
		end = start + maxLines - 1
	}

	selected := lines[start-1 : end]
	return strings.Join(selected, "\n")
}

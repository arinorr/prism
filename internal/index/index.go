// Package index builds a symbol lookup table from source files.
// The index maps symbol names to their locations (file, line range, kind)
// for O(1) lookup. It is built once from pure Go code — no LLM calls.
package index

import (
	"sort"
)

// SymbolKind identifies the type of a symbol declaration.
type SymbolKind string

const (
	KindFunc      SymbolKind = "func"
	KindMethod    SymbolKind = "method"
	KindType      SymbolKind = "type"
	KindInterface SymbolKind = "interface"
	KindClass     SymbolKind = "class"
	KindVariable  SymbolKind = "variable"
)

// Symbol represents a named declaration in a source file.
type Symbol struct {
	Name      string     // identifier name
	File      string     // file path (relative to repo root)
	StartLine int        // first line of the declaration
	EndLine   int        // last line of the declaration
	Kind      SymbolKind // func, method, type, interface, class, variable
	Receiver  string     // receiver type name for methods; empty otherwise
}

// LanguageScanner extracts symbol declarations from source file content.
type LanguageScanner interface {
	Scan(filename string, src []byte) []Symbol
}

// Index is an immutable symbol lookup table built from source files.
// Call Freeze after adding all symbols to prepare it for queries.
type Index struct {
	symbols map[string][]Symbol // name → symbols
	byFile  map[string][]Symbol // file → symbols sorted by StartLine
	frozen  bool
}

// NewIndex creates an empty, mutable index. Call Freeze when done adding.
func NewIndex() *Index {
	return &Index{
		symbols: make(map[string][]Symbol),
		byFile:  make(map[string][]Symbol),
	}
}

// Add inserts a symbol into the index. Must be called before Freeze.
func (idx *Index) Add(sym Symbol) { //nolint:gocritic // hugeParam — Symbol is stored by value in maps, pointer would require copy anyway.
	idx.symbols[sym.Name] = append(idx.symbols[sym.Name], sym)
	idx.byFile[sym.File] = append(idx.byFile[sym.File], sym)
}

// Freeze sorts the byFile entries by StartLine for efficient queries.
// After calling Freeze, Add must not be called.
func (idx *Index) Freeze() {
	for file := range idx.byFile {
		syms := idx.byFile[file]
		sort.Slice(syms, func(i, j int) bool {
			return syms[i].StartLine < syms[j].StartLine
		})
		idx.byFile[file] = syms
	}
	idx.frozen = true
}

// Lookup returns all symbols with the given name. Returns nil if not found.
func (idx *Index) Lookup(name string) []Symbol {
	return idx.symbols[name]
}

// EnclosingScope returns the tightest (smallest span) symbol whose line range
// contains the given line, or nil if no scope encloses that line.
func (idx *Index) EnclosingScope(file string, line int) *Symbol {
	syms := idx.byFile[file]
	var best *Symbol
	for i := range syms {
		if syms[i].StartLine <= line && line <= syms[i].EndLine {
			span := syms[i].EndLine - syms[i].StartLine
			if best == nil || span < (best.EndLine-best.StartLine) {
				best = &syms[i]
			}
		}
	}
	return best
}

// SymbolsInFile returns all symbols declared in the given file,
// sorted by StartLine.
func (idx *Index) SymbolsInFile(file string) []Symbol {
	return idx.byFile[file]
}

// AllFiles returns all file paths that have symbols in the index,
// sorted alphabetically for deterministic output.
func (idx *Index) AllFiles() []string {
	files := make([]string, 0, len(idx.byFile))
	for f := range idx.byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}

// Size returns the total number of symbols in the index.
func (idx *Index) Size() int {
	n := 0
	for _, syms := range idx.symbols {
		n += len(syms)
	}
	return n
}

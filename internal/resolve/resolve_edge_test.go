package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/parse"
)

func TestResolver_MaxReferences(t *testing.T) {
	dir := t.TempDir()

	// Create a file that references many symbols.
	var refs []string
	for i := 0; i < 20; i++ {
		refs = append(refs, string(rune('A'+i)))
	}
	handlerSrc := "package main\n\nfunc Handler() {\n"
	for _, r := range refs {
		handlerSrc += "\t_ = " + r + "()\n"
	}
	handlerSrc += "}\n"
	writeFile(t, dir, "handler.go", handlerSrc)

	// Create referenced files.
	for _, r := range refs {
		writeFile(t, dir, strings.ToLower(r)+".go", "package main\n\nfunc "+r+"() int { return 0 }\n")
	}

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	for _, f := range append(refs, "handler") {
		name := strings.ToLower(f) + ".go"
		src, _ := os.ReadFile(filepath.Join(dir, name))
		for _, sym := range scanner.Scan(name, src) {
			idx.Add(sym)
		}
	}
	idx.Freeze()

	r := NewResolver(idx, dir)
	allFiles := []string{"handler.go"}
	for _, ref := range refs {
		allFiles = append(allFiles, strings.ToLower(ref)+".go")
	}
	r.PreloadFiles(allFiles)

	rc, err := r.Resolve("handler.go", 4)
	if err != nil {
		t.Fatal(err)
	}

	if len(rc.References) > maxReferences {
		t.Errorf("references = %d, should be capped at %d", len(rc.References), maxReferences)
	}
}

func TestResolver_ScopeLineCap(t *testing.T) {
	dir := t.TempDir()

	// Create a very long function.
	var lines []string
	lines = append(lines, "package main", "", "func VeryLong() {")
	for i := 0; i < 200; i++ {
		lines = append(lines, "\t_ = i")
	}
	lines = append(lines, "}")
	writeFile(t, dir, "long.go", strings.Join(lines, "\n"))

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	src, _ := os.ReadFile(filepath.Join(dir, "long.go"))
	for _, sym := range scanner.Scan("long.go", src) {
		idx.Add(sym)
	}
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"long.go"})

	rc, err := r.Resolve("long.go", 50)
	if err != nil {
		t.Fatal(err)
	}

	if rc.EnclosingScope == nil {
		t.Fatal("expected enclosing scope")
	}

	scopeLines := strings.Count(rc.EnclosingScope.Text, "\n") + 1
	if scopeLines > maxScopeLines {
		t.Errorf("scope lines = %d, should be capped at %d", scopeLines, maxScopeLines)
	}
}

func TestResolver_MissingReferenceFileSkipped(t *testing.T) {
	dir := t.TempDir()

	handlerSrc := `package main

func Handler() {
	_ = Missing()
}
`
	writeFile(t, dir, "handler.go", handlerSrc)

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	src, _ := os.ReadFile(filepath.Join(dir, "handler.go"))
	for _, sym := range scanner.Scan("handler.go", src) {
		idx.Add(sym)
	}
	// Add a symbol in a file that doesn't exist on disk.
	idx.Add(parse.Symbol{Name: "Missing", File: "missing.go", StartLine: 1, EndLine: 3, Kind: parse.KindFunc})
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"handler.go"}) // missing.go NOT preloaded

	rc, err := r.Resolve("handler.go", 4)
	if err != nil {
		t.Fatal(err)
	}

	// Should not crash; Missing should be skipped.
	for _, ref := range rc.References {
		if ref.Symbol.Name == "Missing" {
			t.Error("Missing should have been skipped (not in cache)")
		}
	}
}

func TestResolver_PreloadIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package main\nfunc A() {}\n")

	idx := parse.NewIndex()
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"a.go"})
	r.PreloadFiles([]string{"a.go"}) // should not panic or double-read
}

func TestResolver_CollectReferenceFiles_Empty(t *testing.T) {
	idx := parse.NewIndex()
	idx.Freeze()

	r := NewResolver(idx, t.TempDir())
	refs := r.CollectReferenceFiles(nil)
	if len(refs) != 0 {
		t.Errorf("expected 0 reference files, got %d", len(refs))
	}
}

func TestResolver_LineOutOfRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "small.go", "package main\nfunc Tiny() {}\n")

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	src, _ := os.ReadFile(filepath.Join(dir, "small.go"))
	for _, sym := range scanner.Scan("small.go", src) {
		idx.Add(sym)
	}
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"small.go"})

	// Line 999 is way beyond the file.
	rc, err := r.Resolve("small.go", 999)
	if err != nil {
		t.Fatal(err)
	}
	// Should return partial context (no enclosing scope for that line).
	if rc.EnclosingScope != nil {
		t.Error("expected nil enclosing scope for out-of-range line")
	}
}

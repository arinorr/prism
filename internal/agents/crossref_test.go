package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/difflex"
	"github.com/arinorr/prism/internal/parse"
	"github.com/arinorr/prism/internal/resolve"
)

// FilterByIndex.

func TestFilterByIndex_KeepsIndexed(t *testing.T) {
	t.Parallel()
	idx := parse.NewIndex()
	idx.Add(parse.Symbol{Name: "ProcessBatch", File: "worker.go", StartLine: 1, EndLine: 5, Kind: parse.KindFunc})
	idx.Freeze()

	candidates := []difflex.SymbolReference{
		{Name: "ProcessBatch", Kind: difflex.RefCall},
		{Name: "localVar", Kind: difflex.RefCall},
	}

	filtered := FilterByIndex(candidates, idx)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered, got %d", len(filtered))
	}
	if filtered[0].Name != "ProcessBatch" {
		t.Errorf("expected ProcessBatch, got %s", filtered[0].Name)
	}
}

func TestFilterByIndex_EmptyCandidates(t *testing.T) {
	t.Parallel()
	idx := parse.NewIndex()
	idx.Freeze()

	filtered := FilterByIndex(nil, idx)
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered, got %d", len(filtered))
	}
}

func TestFilterByIndex_EmptyIndex(t *testing.T) {
	t.Parallel()
	idx := parse.NewIndex()
	idx.Freeze()

	candidates := []difflex.SymbolReference{
		{Name: "Anything", Kind: difflex.RefCall},
	}

	filtered := FilterByIndex(candidates, idx)
	if len(filtered) != 0 {
		t.Errorf("expected 0 filtered, got %d", len(filtered))
	}
}

// FormatCrossReferences.

func TestFormatCrossReferences_Empty(t *testing.T) {
	t.Parallel()
	if got := FormatCrossReferences(nil); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := FormatCrossReferences([]CrossReference{}); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFormatCrossReferences_ProducesValidBlock(t *testing.T) {
	t.Parallel()
	refs := []CrossReference{{
		Symbol:         parse.Symbol{Name: "HandleRequest", File: "handler.go", Kind: parse.KindFunc, StartLine: 10, EndLine: 25},
		Text:           "func HandleRequest(w http.ResponseWriter, r *http.Request) {\n\t// ...\n}",
		ReferencedFrom: "handler_test.go",
	}}

	got := FormatCrossReferences(refs)

	if !strings.Contains(got, "<cross-references>") {
		t.Error("expected <cross-references> opening tag")
	}
	if !strings.Contains(got, "</cross-references>") {
		t.Error("expected </cross-references> closing tag")
	}
	if !strings.Contains(got, "Referenced from handler_test.go") {
		t.Error("expected ReferencedFrom in output")
	}
	if !strings.Contains(got, "HandleRequest") {
		t.Error("expected symbol name in output")
	}
}

// ResolveCrossReferences.

func TestResolveCrossReferences_NilContext(t *testing.T) {
	t.Parallel()
	refs := ResolveCrossReferences(nil, nil)
	if len(refs) != 0 {
		t.Errorf("expected nil/empty, got %d", len(refs))
	}
}

func TestResolveCrossReferences_NilIndex(t *testing.T) {
	t.Parallel()
	rctx := &ReviewContext{} // Index and Resolver are nil
	refs := ResolveCrossReferences([]ClassifiedFile{{Path: "test.go", Category: PRCategoryTests}}, rctx)
	if len(refs) != 0 {
		t.Errorf("expected nil, got %d", len(refs))
	}
}

func TestResolveCrossReferences_CodeOnlyNoRefs(t *testing.T) {
	t.Parallel()
	idx := parse.NewIndex()
	idx.Freeze()
	rctx := &ReviewContext{
		Index:    idx,
		Resolver: resolve.NewResolver(idx, t.TempDir()),
	}

	codeFiles := []ClassifiedFile{
		{Path: "handler.go", Category: PRCategoryCode, Diff: "+func Handler() {}"},
	}

	refs := ResolveCrossReferences(codeFiles, rctx)
	if len(refs) != 0 {
		t.Errorf("code-only files should produce no cross-refs, got %d", len(refs))
	}
}

func TestResolveCrossReferences_TestRefsCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write source files.
	writeTestSourceFile(t, dir, "handler.go", `package main

func HandleRequest(data string) error {
	return nil
}
`)

	// Build index from the source.
	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	src, _ := os.ReadFile(filepath.Join(dir, "handler.go"))
	for _, sym := range scanner.Scan("handler.go", src) {
		idx.Add(sym)
	}
	idx.Freeze()

	resolver := resolve.NewResolver(idx, dir)
	resolver.PreloadFiles([]string{"handler.go"})

	rctx := &ReviewContext{
		Index:    idx,
		Resolver: resolver,
	}

	// Test file diff that references HandleRequest.
	testFiles := []ClassifiedFile{{
		Path:     "handler_test.go",
		Category: PRCategoryTests,
		Diff:     "+\tresult := HandleRequest(\"data\")\n+\tassert.NoError(t, result)\n",
	}}

	refs := ResolveCrossReferences(testFiles, rctx)

	if len(refs) == 0 {
		t.Fatal("expected at least 1 cross-reference")
	}

	found := false
	for _, ref := range refs {
		if ref.Symbol.Name == "HandleRequest" {
			found = true
			if ref.ReferencedFrom != "handler_test.go" {
				t.Errorf("ReferencedFrom = %q, want handler_test.go", ref.ReferencedFrom)
			}
			if ref.Text == "" {
				t.Error("expected non-empty definition text")
			}
		}
	}
	if !found {
		t.Error("expected HandleRequest in cross-references")
	}
}

func TestResolveCrossReferences_Cap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create many functions.
	var src strings.Builder
	src.WriteString("package main\n\n")
	for i := 0; i < 20; i++ {
		name := string(rune('A'+i)) + "Func"
		src.WriteString("func " + name + "() {}\n\n")
	}
	writeTestSourceFile(t, dir, "funcs.go", src.String())

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	data, _ := os.ReadFile(filepath.Join(dir, "funcs.go"))
	for _, sym := range scanner.Scan("funcs.go", data) {
		idx.Add(sym)
	}
	idx.Freeze()

	resolver := resolve.NewResolver(idx, dir)
	resolver.PreloadFiles([]string{"funcs.go"})

	rctx := &ReviewContext{Index: idx, Resolver: resolver}

	// Diff that references all 20 functions.
	var diffLines strings.Builder
	for i := 0; i < 20; i++ {
		name := string(rune('A'+i)) + "Func"
		diffLines.WriteString("+\t" + name + "()\n")
	}

	testFiles := []ClassifiedFile{{
		Path:     "test.go",
		Category: PRCategoryTests,
		Diff:     diffLines.String(),
	}}

	refs := ResolveCrossReferences(testFiles, rctx)

	if len(refs) > maxCrossRefs {
		t.Errorf("expected max %d cross-refs, got %d", maxCrossRefs, len(refs))
	}
}

func TestResolveCrossReferences_DedupPreferChanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestSourceFile(t, dir, "handler.go", `package main
func HandleRequest() {}
`)

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	data, _ := os.ReadFile(filepath.Join(dir, "handler.go"))
	for _, sym := range scanner.Scan("handler.go", data) {
		idx.Add(sym)
	}
	idx.Freeze()

	resolver := resolve.NewResolver(idx, dir)
	resolver.PreloadFiles([]string{"handler.go"})

	rctx := &ReviewContext{Index: idx, Resolver: resolver}

	// Two test files reference HandleRequest: one from context, one from changed line.
	testFiles := []ClassifiedFile{
		{Path: "test_a.go", Category: PRCategoryTests, Diff: " \tHandleRequest()\n"},
		{Path: "test_b.go", Category: PRCategoryTests, Diff: "+\tHandleRequest()\n"},
	}

	refs := ResolveCrossReferences(testFiles, rctx)

	// Should only have one HandleRequest ref, from test_b.go (changed line).
	count := 0
	for _, ref := range refs {
		if ref.Symbol.Name == "HandleRequest" {
			count++
			if ref.ReferencedFrom != "test_b.go" {
				t.Errorf("expected ReferencedFrom=test_b.go (changed line), got %s", ref.ReferencedFrom)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 HandleRequest ref (deduped), got %d", count)
	}
}

func TestResolveCrossReferences_SkipsDeclarations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	writeTestSourceFile(t, dir, "handler.go", `package main
func HandleRequest() {}
`)

	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	data, _ := os.ReadFile(filepath.Join(dir, "handler.go"))
	for _, sym := range scanner.Scan("handler.go", data) {
		idx.Add(sym)
	}
	idx.Freeze()

	resolver := resolve.NewResolver(idx, dir)
	resolver.PreloadFiles([]string{"handler.go"})

	rctx := &ReviewContext{Index: idx, Resolver: resolver}

	// Test file declares a function (not a reference to cross-resolve).
	testFiles := []ClassifiedFile{{
		Path:     "test.go",
		Category: PRCategoryTests,
		Diff:     "+func TestHelper() {}\n",
	}}

	refs := ResolveCrossReferences(testFiles, rctx)

	for _, ref := range refs {
		if ref.Symbol.Name == "TestHelper" {
			t.Error("declarations should not become cross-references")
		}
	}
}

// TestResolveCrossReferences_SecondaryRefsPreloaded verifies that the
// preload-all-files strategy resolves secondary references. A test file
// calls HandleRequest, which references Config (in a separate file).
// Without preloading all indexed files, Config's file would be "not in cache".
func TestResolveCrossReferences_SecondaryRefsPreloaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// handler.go defines HandleRequest which uses Config.
	writeTestSourceFile(t, dir, "handler.go", `package main

func HandleRequest(cfg Config) error {
	if cfg.Port == 0 {
		return fmt.Errorf("no port")
	}
	return nil
}
`)
	// types.go defines Config — a secondary reference.
	writeTestSourceFile(t, dir, "types.go", `package main

type Config struct {
	Host string
	Port int
}
`)

	// Build index from both files.
	idx := parse.NewIndex()
	scanner := parse.GoScanner{}
	for _, f := range []string{"handler.go", "types.go"} {
		src, _ := os.ReadFile(filepath.Join(dir, f))
		for _, sym := range scanner.Scan(f, src) {
			idx.Add(sym)
		}
	}
	idx.Freeze()

	resolver := resolve.NewResolver(idx, dir)

	rctx := &ReviewContext{
		Index:    idx,
		Resolver: resolver,
	}

	// Test file diff that calls HandleRequest.
	testFiles := []ClassifiedFile{{
		Path:     "handler_test.go",
		Category: PRCategoryTests,
		Diff:     "+\terr := HandleRequest(cfg)\n+\tassert.NoError(t, err)\n",
	}}

	refs := ResolveCrossReferences(testFiles, rctx)

	// HandleRequest should be resolved with its definition text.
	found := false
	for _, ref := range refs {
		if ref.Symbol.Name == "HandleRequest" {
			found = true
			// The definition text should include "Config" because the resolver
			// can now read types.go (preloaded via AllFiles).
			if !strings.Contains(ref.Text, "Config") {
				t.Error("expected HandleRequest definition to reference Config (secondary ref should be preloaded)")
			}
		}
	}
	if !found {
		t.Error("expected HandleRequest in cross-references")
	}
}

// TestAllFiles_Deterministic verifies AllFiles returns sorted, deterministic output.
func TestAllFiles_Deterministic(t *testing.T) {
	t.Parallel()
	idx := parse.NewIndex()
	idx.Add(parse.Symbol{Name: "C", File: "c.go", Kind: parse.KindFunc})
	idx.Add(parse.Symbol{Name: "A", File: "a.go", Kind: parse.KindFunc})
	idx.Add(parse.Symbol{Name: "B", File: "b.go", Kind: parse.KindFunc})
	idx.Freeze()

	files := idx.AllFiles()
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}
	if files[0] != "a.go" || files[1] != "b.go" || files[2] != "c.go" {
		t.Errorf("expected sorted [a.go b.go c.go], got %v", files)
	}

	// Run again — should be identical.
	files2 := idx.AllFiles()
	for i := range files {
		if files[i] != files2[i] {
			t.Errorf("AllFiles not deterministic: run1[%d]=%s, run2[%d]=%s", i, files[i], i, files2[i])
		}
	}
}

func writeTestSourceFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

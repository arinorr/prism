package resolve

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arinorr/prism/internal/index"
)

func TestResolver_Resolve(t *testing.T) {
	dir := t.TempDir()

	// Create source files.
	handlerSrc := `package main

import "fmt"

func HandleRequest(cfg Config) error {
	if cfg.Port == 0 {
		return fmt.Errorf("invalid port")
	}
	data := ProcessData(cfg)
	return Send(data)
}

func Send(data []byte) error {
	return nil
}
`
	typesSrc := `package main

type Config struct {
	Host string
	Port int
}
`
	processSrc := `package main

func ProcessData(cfg Config) []byte {
	return []byte(cfg.Host)
}
`

	writeFile(t, dir, "handler.go", handlerSrc)
	writeFile(t, dir, "types.go", typesSrc)
	writeFile(t, dir, "process.go", processSrc)

	// Build index.
	idx := index.NewIndex()
	goScanner := index.GoScanner{}

	for _, f := range []string{"handler.go", "types.go", "process.go"} {
		src, _ := os.ReadFile(filepath.Join(dir, f))
		for _, sym := range goScanner.Scan(f, src) {
			idx.Add(sym)
		}
	}
	idx.Freeze()

	// Create resolver and preload.
	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"handler.go", "types.go", "process.go"})

	// Resolve a finding at line 6 of handler.go (inside HandleRequest).
	rc, err := r.Resolve("handler.go", 6)
	if err != nil {
		t.Fatal(err)
	}

	if rc.EnclosingScope == nil {
		t.Fatal("expected enclosing scope, got nil")
	}
	if rc.EnclosingScope.Symbol.Name != "HandleRequest" {
		t.Errorf("enclosing scope = %q, want HandleRequest", rc.EnclosingScope.Symbol.Name)
	}

	// Should have references to Config and ProcessData (Send too, but order depends on text scanning).
	refNames := make(map[string]bool)
	for _, ref := range rc.References {
		refNames[ref.Symbol.Name] = true
	}
	if !refNames["Config"] {
		t.Error("expected reference to Config")
	}
	if !refNames["ProcessData"] {
		t.Error("expected reference to ProcessData")
	}
}

func TestResolver_FileNotPreloaded(t *testing.T) {
	idx := index.NewIndex()
	idx.Freeze()

	r := NewResolver(idx, t.TempDir())
	_, err := r.Resolve("missing.go", 1)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestResolver_NoEnclosingScope(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bare.go", "package main\n\n// just a comment\n")

	idx := index.NewIndex()
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"bare.go"})

	rc, err := r.Resolve("bare.go", 3)
	if err != nil {
		t.Fatal(err)
	}
	if rc.EnclosingScope != nil {
		t.Error("expected nil enclosing scope for bare file")
	}
}

func TestResolver_CollectReferenceFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "a.go", `package main

func DoWork() {
	cfg := LoadConfig()
	Process(cfg)
}
`)
	writeFile(t, dir, "b.go", `package main

func LoadConfig() Config { return Config{} }
`)
	writeFile(t, dir, "c.go", `package main

type Config struct{ Port int }

func Process(c Config) {}
`)

	idx := index.NewIndex()
	goScanner := index.GoScanner{}
	for _, f := range []string{"a.go", "b.go", "c.go"} {
		src, _ := os.ReadFile(filepath.Join(dir, f))
		for _, sym := range goScanner.Scan(f, src) {
			idx.Add(sym)
		}
	}
	idx.Freeze()

	r := NewResolver(idx, dir)
	r.PreloadFiles([]string{"a.go"})

	refs := r.CollectReferenceFiles([]string{"a.go"})
	refSet := make(map[string]bool)
	for _, f := range refs {
		refSet[f] = true
	}

	// DoWork references LoadConfig (b.go) and Process (c.go).
	if !refSet["b.go"] {
		t.Error("expected b.go in reference files")
	}
	if !refSet["c.go"] {
		t.Error("expected c.go in reference files")
	}
}

func TestExtractLines(t *testing.T) {
	src := []byte("line1\nline2\nline3\nline4\nline5")

	got := extractLines(src, 2, 4, 100)
	if got != "line2\nline3\nline4" {
		t.Errorf("extractLines(2,4) = %q", got)
	}

	// Test cap.
	got = extractLines(src, 1, 5, 2)
	if got != "line1\nline2" {
		t.Errorf("extractLines(1,5,cap=2) = %q", got)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

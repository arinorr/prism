package parse

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIndex_EnclosingScope(t *testing.T) {
	idx := NewIndex()
	idx.Add(Symbol{Name: "outer", File: "a.go", StartLine: 1, EndLine: 50, Kind: KindFunc})
	idx.Add(Symbol{Name: "inner", File: "a.go", StartLine: 10, EndLine: 20, Kind: KindFunc})
	idx.Add(Symbol{Name: "other", File: "a.go", StartLine: 55, EndLine: 60, Kind: KindFunc})
	idx.Freeze()

	tests := []struct {
		line int
		want string
	}{
		{5, "outer"},  // inside outer but before inner
		{15, "inner"}, // inside inner (tightest)
		{30, "outer"}, // inside outer but after inner
		{57, "other"}, // inside other
		{100, ""},     // outside all
	}

	for _, tt := range tests {
		sym := idx.EnclosingScope("a.go", tt.line)
		got := ""
		if sym != nil {
			got = sym.Name
		}
		if got != tt.want {
			t.Errorf("EnclosingScope(a.go, %d) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestIndex_Lookup(t *testing.T) {
	idx := NewIndex()
	idx.Add(Symbol{Name: "Foo", File: "a.go", StartLine: 1, EndLine: 5, Kind: KindType})
	idx.Add(Symbol{Name: "Foo", File: "b.go", StartLine: 10, EndLine: 15, Kind: KindFunc})
	idx.Add(Symbol{Name: "Bar", File: "a.go", StartLine: 20, EndLine: 25, Kind: KindInterface})
	idx.Freeze()

	foos := idx.Lookup("Foo")
	if len(foos) != 2 {
		t.Errorf("Lookup(Foo) returned %d symbols, want 2", len(foos))
	}

	bars := idx.Lookup("Bar")
	if len(bars) != 1 {
		t.Errorf("Lookup(Bar) returned %d symbols, want 1", len(bars))
	}

	nope := idx.Lookup("Nope")
	if len(nope) != 0 {
		t.Errorf("Lookup(Nope) returned %d symbols, want 0", len(nope))
	}
}

func TestIndex_Size(t *testing.T) {
	idx := NewIndex()
	if idx.Size() != 0 {
		t.Errorf("empty index size = %d, want 0", idx.Size())
	}
	idx.Add(Symbol{Name: "A", File: "a.go", Kind: KindFunc})
	idx.Add(Symbol{Name: "B", File: "a.go", Kind: KindType})
	if idx.Size() != 2 {
		t.Errorf("index size = %d, want 2", idx.Size())
	}
}

func TestBuild(t *testing.T) {
	dir := t.TempDir()

	// Create a Go file.
	goContent := []byte(`package main

func Hello() string {
	return "hello"
}

type Config struct {
	Port int
}
`)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), goContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a TS file.
	tsContent := []byte(`export function greet(name: string): string {
  return "hi " + name;
}

export interface AppConfig {
  debug: boolean;
}
`)
	if err := os.WriteFile(filepath.Join(dir, "app.ts"), tsContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a file that should be ignored (no matching language).
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Readme"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a node_modules dir that should be skipped.
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "index.ts"), []byte("export function skip() {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(context.Background(), dir, []string{"go", "typescript"})
	if err != nil {
		t.Fatal(err)
	}

	// Should have: Hello, Config (from Go) + greet, AppConfig (from TS) = 4
	if idx.Size() != 4 {
		t.Errorf("index size = %d, want 4", idx.Size())
		for file, syms := range idx.byFile {
			for _, s := range syms {
				t.Logf("  %s: %s (%s) %d-%d", file, s.Name, s.Kind, s.StartLine, s.EndLine)
			}
		}
	}

	// node_modules should have been skipped.
	if syms := idx.Lookup("skip"); len(syms) != 0 {
		t.Errorf("node_modules file was indexed: %+v", syms)
	}
}

func TestBuild_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := Build(ctx, t.TempDir(), []string{"go"})
	if err == nil {
		t.Error("expected error from canceled context, got nil")
	}
}

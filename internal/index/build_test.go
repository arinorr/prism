package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()

	// Create .git directory with a Go file.
	gitDir := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "pre-commit.go"), []byte("package hooks\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a real Go file.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(context.Background(), dir, []string{"go"})
	if err != nil {
		t.Fatal(err)
	}

	if syms := idx.Lookup("Run"); len(syms) != 0 {
		t.Error(".git directory should be skipped")
	}
	if syms := idx.Lookup("Main"); len(syms) == 0 {
		t.Error("main.go should be indexed")
	}
}

func TestBuild_LargeFileSkipped(t *testing.T) {
	dir := t.TempDir()

	// Create a file larger than maxFileSize.
	large := make([]byte, maxFileSize+1)
	copy(large, []byte("package main\nfunc Huge() {}\n"))
	if err := os.WriteFile(filepath.Join(dir, "huge.go"), large, 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(context.Background(), dir, []string{"go"})
	if err != nil {
		t.Fatal(err)
	}

	if syms := idx.Lookup("Huge"); len(syms) != 0 {
		t.Error("large file should be skipped")
	}
}

func TestBuild_OnlyRequestedLanguages(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc GoFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.ts"), []byte("export function tsFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Only request Go.
	idx, err := Build(context.Background(), dir, []string{"go"})
	if err != nil {
		t.Fatal(err)
	}

	if syms := idx.Lookup("GoFunc"); len(syms) == 0 {
		t.Error("Go file should be indexed")
	}
	if syms := idx.Lookup("tsFunc"); len(syms) != 0 {
		t.Error("TS file should not be indexed when only Go requested")
	}
}

func TestBuild_NestedDirectories(t *testing.T) {
	dir := t.TempDir()

	nested := filepath.Join(dir, "pkg", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.go"), []byte("package sub\nfunc DeepFunc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(context.Background(), dir, []string{"go"})
	if err != nil {
		t.Fatal(err)
	}

	if syms := idx.Lookup("DeepFunc"); len(syms) == 0 {
		t.Error("deeply nested file should be indexed")
	}

	// Check relative path.
	syms := idx.Lookup("DeepFunc")
	if syms[0].File != filepath.Join("pkg", "sub", "deep.go") {
		t.Errorf("file path = %q, want relative path", syms[0].File)
	}
}

func TestBuild_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	idx, err := Build(context.Background(), dir, []string{"go", "typescript"})
	if err != nil {
		t.Fatal(err)
	}

	if idx.Size() != 0 {
		t.Errorf("empty directory should produce empty index, got %d symbols", idx.Size())
	}
}

func TestBuild_MixedLanguages(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "server.go"), []byte("package main\ntype Server struct{}\nfunc (s *Server) Start() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "client.ts"), []byte("export class Client {\n  connect() { return true; }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "utils.jsx"), []byte("export function render() { return <div/>; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := Build(context.Background(), dir, []string{"go", "typescript"})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Server", "Start", "Client", "connect", "render"} {
		if syms := idx.Lookup(name); len(syms) == 0 {
			t.Errorf("expected symbol %q in mixed-language index", name)
		}
	}
}

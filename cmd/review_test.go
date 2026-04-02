package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename_SimpleNumber(t *testing.T) {
	got := sanitizeFilename("42")
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_URLStyleRef(t *testing.T) {
	got := sanitizeFilename("https://github.com/org/repo/pull/42")
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_PathTraversal(t *testing.T) {
	got := sanitizeFilename("../../etc/passwd")
	if got != "passwd" {
		t.Errorf("expected 'passwd', got %q", got)
	}
}

func TestSanitizeFilename_DoubleDots(t *testing.T) {
	got := sanitizeFilename("..42")
	if got != "42" {
		t.Errorf("expected '42', got %q", got)
	}
}

func TestSanitizeFilename_Tilde(t *testing.T) {
	got := sanitizeFilename("~root")
	if got != "root" {
		t.Errorf("expected 'root', got %q", got)
	}
}

func TestWriteToFile_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "report.md")
	err := writeToFile("hello world", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read back: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestWriteToFile_InvalidPath(t *testing.T) {
	err := writeToFile("content", "/dev/null/impossible/path.txt")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

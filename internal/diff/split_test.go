package diff

import "testing"

func TestSplitToMap_ValidDiff(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,2 +1,3 @@
 package main
+func main() {}
`

	m := SplitToMap(raw)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if _, ok := m["handler.go"]; !ok {
		t.Error("expected handler.go in map")
	}
	if _, ok := m["main.go"]; !ok {
		t.Error("expected main.go in map")
	}
}

func TestSplitToMap_Empty(t *testing.T) {
	t.Parallel()
	m := SplitToMap("")
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}
}

func TestSplitToMap_MalformedNoMarker(t *testing.T) {
	t.Parallel()
	m := SplitToMap("just some text\nno diff markers\n")
	// No "diff --git" marker → extractFilePath returns "" → skipped.
	if len(m) != 0 {
		t.Errorf("expected 0 entries for malformed diff, got %d", len(m))
	}
}

func TestSplitToMap_MissingFilePath(t *testing.T) {
	t.Parallel()
	// "diff --git" with insufficient fields.
	m := SplitToMap("diff --git\n+some content\n")
	if len(m) != 0 {
		t.Errorf("expected 0 entries when file path can't be extracted, got %d", len(m))
	}
}

func TestSplitToMap_SingleFile(t *testing.T) {
	t.Parallel()
	raw := `diff --git a/only.go b/only.go
+package main
`
	m := SplitToMap(raw)
	if len(m) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m))
	}
	if _, ok := m["only.go"]; !ok {
		t.Error("expected only.go")
	}
}

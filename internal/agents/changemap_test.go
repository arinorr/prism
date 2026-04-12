package agents

import (
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/index"
)

func TestBuildChangeMap_ModifiedFunction(t *testing.T) {
	t.Parallel()

	idx := index.NewIndex()
	idx.Add(index.Symbol{Name: "HandleRequest", File: "handler.go", StartLine: 5, EndLine: 15, Kind: index.KindFunc})
	idx.Freeze()

	files := []ClassifiedFile{{
		Path:     "handler.go",
		Category: PRCategoryCode,
		Diff: `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -5,7 +5,8 @@ func HandleRequest(data string) error {
 	if data == "" {
-		return fmt.Errorf("empty")
+		return fmt.Errorf("empty data")
+	}
+	log.Println("handled")
 	return nil
 }`,
	}}

	cm := BuildChangeMap(files, idx)

	if !cm.Modified[SymbolKey{File: "handler.go", Name: "HandleRequest"}] {
		t.Error("HandleRequest should be modified")
	}
	if len(cm.Added) != 0 {
		t.Errorf("expected 0 added, got %d", len(cm.Added))
	}
}

func TestBuildChangeMap_NewFunction(t *testing.T) {
	t.Parallel()

	idx := index.NewIndex()
	idx.Freeze() // empty index — nothing pre-exists

	files := []ClassifiedFile{{
		Path:     "helper.go",
		Category: PRCategoryCode,
		Diff: `diff --git a/helper.go b/helper.go
--- /dev/null
+++ b/helper.go
@@ -0,0 +1,5 @@
+package main
+
+func NewHelper() string {
+	return "help"
+}`,
	}}

	cm := BuildChangeMap(files, idx)

	if !cm.Added[SymbolKey{File: "helper.go", Name: "NewHelper"}] {
		t.Error("NewHelper should be added")
	}
	if len(cm.Modified) != 0 {
		t.Errorf("expected 0 modified, got %d", len(cm.Modified))
	}
}

func TestBuildChangeMap_NestedDeclaration(t *testing.T) {
	t.Parallel()

	idx := index.NewIndex()
	idx.Add(index.Symbol{Name: "HandleRequest", File: "handler.go", StartLine: 3, EndLine: 10, Kind: index.KindFunc})
	idx.Freeze()

	files := []ClassifiedFile{{
		Path:     "handler.go",
		Category: PRCategoryCode,
		Diff: `diff --git a/handler.go b/handler.go
@@ -3,6 +3,10 @@ func HandleRequest() {
 	data := getData()
+	func innerHelper() {
+		log.Println("inner")
+	}
+	innerHelper()
 	return
 }`,
	}}

	cm := BuildChangeMap(files, idx)

	if !cm.Added[SymbolKey{File: "handler.go", Name: "innerHelper"}] {
		t.Error("innerHelper should be added")
	}
	if !cm.Modified[SymbolKey{File: "handler.go", Name: "HandleRequest"}] {
		t.Error("HandleRequest should be modified")
	}
}

func TestBuildChangeMap_AllNewFile(t *testing.T) {
	t.Parallel()

	idx := index.NewIndex()
	idx.Freeze()

	files := []ClassifiedFile{{
		Path:     "new.go",
		Category: PRCategoryCode,
		Diff: `diff --git a/new.go b/new.go
--- /dev/null
+++ b/new.go
@@ -0,0 +1,7 @@
+package main
+
+func FuncA() {}
+
+func FuncB() {}`,
	}}

	cm := BuildChangeMap(files, idx)

	if !cm.Added[SymbolKey{File: "new.go", Name: "FuncA"}] {
		t.Error("FuncA should be added")
	}
	if !cm.Added[SymbolKey{File: "new.go", Name: "FuncB"}] {
		t.Error("FuncB should be added")
	}
	if len(cm.Modified) != 0 {
		t.Errorf("new file should have 0 modified, got %d", len(cm.Modified))
	}
}

func TestBuildChangeMap_SameNameDifferentFiles(t *testing.T) {
	t.Parallel()

	idx := index.NewIndex()
	idx.Add(index.Symbol{Name: "Init", File: "cmd/server.go", StartLine: 1, EndLine: 5, Kind: index.KindFunc})
	idx.Add(index.Symbol{Name: "Init", File: "internal/db/db.go", StartLine: 1, EndLine: 5, Kind: index.KindFunc})
	idx.Freeze()

	files := []ClassifiedFile{
		{
			Path: "cmd/server.go", Category: PRCategoryCode,
			Diff: "diff --git a/cmd/server.go b/cmd/server.go\n@@ -1,3 +1,4 @@ func Init() {\n \tsetup()\n+\tlog.Println(\"init\")\n }",
		},
		{
			Path: "internal/db/db.go", Category: PRCategoryCode,
			Diff: "diff --git a/internal/db/db.go b/internal/db/db.go\n@@ -1,3 +1,3 @@ func Init() {\n-\tconnect()\n+\tconnectWithRetry()\n }",
		},
	}

	cm := BuildChangeMap(files, idx)

	if !cm.Modified[SymbolKey{File: "cmd/server.go", Name: "Init"}] {
		t.Error("Init in cmd/server.go should be modified")
	}
	if !cm.Modified[SymbolKey{File: "internal/db/db.go", Name: "Init"}] {
		t.Error("Init in internal/db/db.go should be modified")
	}
}

func TestChangeMap_Status(t *testing.T) {
	t.Parallel()

	cm := &ChangeMap{
		Added:    map[SymbolKey]bool{{File: "a.go", Name: "New"}: true},
		Modified: map[SymbolKey]bool{{File: "b.go", Name: "Old"}: true},
	}

	if cm.Status("a.go", "New") != SymbolAdded {
		t.Error("expected added")
	}
	if cm.Status("b.go", "Old") != SymbolModified {
		t.Error("expected modified")
	}
	if cm.Status("c.go", "Other") != SymbolExisting {
		t.Error("expected existing")
	}
}

func TestSymbolStatus_Values(t *testing.T) {
	t.Parallel()
	if string(SymbolAdded) != "new in this PR" {
		t.Errorf("SymbolAdded = %q", SymbolAdded)
	}
	if string(SymbolModified) != "modified in this PR" {
		t.Errorf("SymbolModified = %q", SymbolModified)
	}
	if string(SymbolExisting) != "pre-existing" {
		t.Errorf("SymbolExisting = %q", SymbolExisting)
	}
}

// parseChangedLines tests.

func TestParseChangedLines_MultiHunk(t *testing.T) {
	t.Parallel()
	diff := `@@ -5,3 +5,4 @@ func A() {
 	existing()
+	added()
 }
@@ -20,3 +21,4 @@ func B() {
 	old()
+	new()
 }`

	changed := parseChangedLines(diff)

	// First hunk: +5 means new file starts at line 5. Context=5, added=6, context=7.
	if !changed[6] {
		t.Error("line 6 (first hunk added line) should be changed")
	}
	// Second hunk: +21 means new file starts at line 21. Context=21, added=22, context=23.
	if !changed[22] {
		t.Error("line 22 (second hunk added line) should be changed")
	}
}

func TestParseChangedLines_RemovedLines(t *testing.T) {
	t.Parallel()
	diff := `@@ -1,4 +1,3 @@
 line1
-removed
 line2
 line3`

	changed := parseChangedLines(diff)

	// Removed line should be marked as changed but NOT increment newLine.
	if !changed[2] {
		t.Error("removed line at position 2 should be marked as changed")
	}
}

func TestParseChangedLines_EmptyDiff(t *testing.T) {
	t.Parallel()
	changed := parseChangedLines("")
	if len(changed) != 0 {
		t.Errorf("expected 0 changed lines, got %d", len(changed))
	}
}

func TestParseChangedLines_NoHunks(t *testing.T) {
	t.Parallel()
	diff := `diff --git a/file.go b/file.go
--- a/file.go
+++ b/file.go`

	changed := parseChangedLines(diff)
	if len(changed) != 0 {
		t.Errorf("expected 0 changed lines for diff with no hunks, got %d", len(changed))
	}
}

// parseHunkNewStart.

func TestParseHunkNewStart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"standard", "@@ -10,5 +15,7 @@", 15},
		{"no count", "@@ -10 +20 @@", 20},
		{"with context", "@@ -10,5 +15,7 @@ func Foo()", 15},
		{"line 1", "@@ -0,0 +1,5 @@", 1},
		{"no plus", "@@ -10,5 @@", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseHunkNewStart(tt.header)
			if got != tt.want {
				t.Errorf("parseHunkNewStart(%q) = %d, want %d", tt.header, got, tt.want)
			}
		})
	}
}

// rangeOverlapsChanges.

func TestRangeOverlapsChanges(t *testing.T) {
	t.Parallel()
	changed := map[int]bool{5: true, 10: true, 15: true}

	tests := []struct {
		name       string
		start, end int
		want       bool
	}{
		{"overlaps start", 3, 7, true},
		{"overlaps middle", 9, 11, true},
		{"overlaps end", 14, 16, true},
		{"no overlap before", 1, 3, false},
		{"no overlap after", 16, 20, false},
		{"exact match", 5, 5, true},
		{"empty range", 6, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := rangeOverlapsChanges(tt.start, tt.end, changed)
			if got != tt.want {
				t.Errorf("rangeOverlapsChanges(%d, %d) = %v, want %v", tt.start, tt.end, got, tt.want)
			}
		})
	}
}

// FormatScopeHints.

func TestFormatScopeHints_Nil(t *testing.T) {
	t.Parallel()
	var cm *ChangeMap
	if got := cm.FormatScopeHints(); got != "" {
		t.Errorf("nil ChangeMap should return empty, got %q", got)
	}
}

func TestFormatScopeHints_Empty(t *testing.T) {
	t.Parallel()
	cm := &ChangeMap{
		Modified: map[SymbolKey]bool{},
		Added:    map[SymbolKey]bool{},
	}
	if got := cm.FormatScopeHints(); got != "" {
		t.Errorf("empty ChangeMap should return empty, got %q", got)
	}
}

func TestFormatScopeHints_WithData(t *testing.T) {
	t.Parallel()
	cm := &ChangeMap{
		Modified: map[SymbolKey]bool{{File: "handler.go", Name: "Handle"}: true},
		Added:    map[SymbolKey]bool{{File: "new.go", Name: "NewFunc"}: true},
	}

	got := cm.FormatScopeHints()

	if !strings.Contains(got, "Handle (handler.go)") {
		t.Error("expected modified symbol with file")
	}
	if !strings.Contains(got, "NewFunc (new.go)") {
		t.Error("expected added symbol with file")
	}
	if !strings.Contains(got, "pre-existing") {
		t.Error("expected pre-existing note")
	}
}

func TestBuildChangeMap_EmptyDiff(t *testing.T) {
	t.Parallel()
	idx := index.NewIndex()
	idx.Freeze()

	files := []ClassifiedFile{{Path: "a.go", Category: PRCategoryCode, Diff: ""}}
	cm := BuildChangeMap(files, idx)

	if len(cm.Added) != 0 || len(cm.Modified) != 0 {
		t.Error("empty diff should produce empty change map")
	}
}

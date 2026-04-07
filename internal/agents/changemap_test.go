package agents

import (
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
	// Verify the display strings are correct.
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

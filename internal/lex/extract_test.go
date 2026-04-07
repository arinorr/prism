package lex

import "testing"

func TestExtractCandidates_SingleLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		diff     string
		wantRefs []SymbolReference
	}{
		{
			"function call",
			"+\tresult := ProcessBatch(data)",
			[]SymbolReference{{Name: "ProcessBatch", Kind: RefCall, InChange: true}},
		},
		{
			"qualified call",
			" \tcfg := config.Load()",
			[]SymbolReference{{Name: "Load", Kind: RefCall, InChange: false}},
		},
		{
			"type annotation",
			"+const x: SomeType = value",
			[]SymbolReference{{Name: "SomeType", Kind: RefType, InChange: true}},
		},
		{
			"declaration func",
			"+func NewHelper() {",
			[]SymbolReference{{Name: "NewHelper", Kind: RefDeclaration, InChange: true}},
		},
		{
			"declaration type",
			"+type Config struct {",
			[]SymbolReference{{Name: "Config", Kind: RefDeclaration, InChange: true}},
		},
		{
			"declaration class",
			"+export class UserService {",
			[]SymbolReference{{Name: "UserService", Kind: RefDeclaration, InChange: true}},
		},
		{
			"declaration function (TS)",
			"+export function handleRequest() {",
			[]SymbolReference{{Name: "handleRequest", Kind: RefDeclaration, InChange: true}},
		},
		{
			"constructor",
			"+\trouter := new Router()",
			[]SymbolReference{{Name: "Router", Kind: RefCall, InChange: true}},
		},
		{
			"string contents ignored",
			`+name := "ProcessBatch"`,
			nil, // ProcessBatch is inside a string
		},
		{
			"comment ignored",
			"+// calls ProcessBatch",
			nil, // entire line is a comment
		},
		{
			"keyword not a ref",
			"+\tif err != nil {",
			nil,
		},
		{
			"context line (not change)",
			" \tresult := Process(data)",
			[]SymbolReference{{Name: "Process", Kind: RefCall, InChange: false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			refs := ExtractCandidates(tt.diff)
			assertRefs(t, refs, tt.wantRefs)
		})
	}
}

func TestExtractCandidates_DiffMetadataSkipped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		diff string
	}{
		{"hunk header", "@@ -10,5 +10,7 @@ func Foo()"},
		{"no-newline marker", `\ No newline at end of file`},
		{"diff header", "diff --git a/handler.go b/handler.go"},
		{"minus header", "--- a/handler.go"},
		{"plus header", "+++ b/handler.go"},
		{"index line", "index abc123..def456 100644"},
		{"empty line", ""},
		{"empty + line", "+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			refs := ExtractCandidates(tt.diff)
			if len(refs) != 0 {
				t.Errorf("expected 0 refs for %q, got %d: %+v", tt.diff, len(refs), refs)
			}
		})
	}
}

func TestExtractCandidates_MultiHunkDiff(t *testing.T) {
	t.Parallel()

	// Golden-style test: realistic multi-hunk diff with various patterns.
	diff := `diff --git a/handler_test.go b/handler_test.go
--- a/handler_test.go
+++ b/handler_test.go
@@ -10,5 +10,12 @@ func TestHandler(t *testing.T) {
 	srv := NewServer()
-	resp := srv.Handle(req)
+	resp := srv.HandleRequest(req)
+	if resp.StatusCode != 200 {
+		t.Errorf("got %d", resp.StatusCode)
+	}
+	data := ParseResponse(resp)
+	cfg := config.LoadConfig()
 }
@@ -20,3 +27,5 @@ func TestOther(t *testing.T) {
 	// existing test
+	result := ProcessBatch(items)
+	assert.NotNil(t, result)
 }`

	refs := ExtractCandidates(diff)

	// Should find: HandleRequest (call, changed), ParseResponse (call, changed),
	// LoadConfig (qualified call, changed), ProcessBatch (call, changed),
	// NewServer (call, context), Handle (qualified call, removed)
	wantNames := map[string]bool{
		"HandleRequest": true,
		"ParseResponse": true,
		"LoadConfig":    true,
		"ProcessBatch":  true,
		"NewServer":     true,
	}

	for _, ref := range refs {
		delete(wantNames, ref.Name)
	}
	for missing := range wantNames {
		t.Errorf("expected ref to %q not found", missing)
	}
}

func TestExtractCandidates_Dedup(t *testing.T) {
	t.Parallel()

	diff := `+	Process(a)
+	Process(b)
+	Process(c)`

	refs := ExtractCandidates(diff)

	count := 0
	for _, ref := range refs {
		if ref.Name == "Process" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Process should appear once (deduped), got %d", count)
	}
}

func assertRefs(t *testing.T, got []SymbolReference, want []SymbolReference) {
	t.Helper()
	if want == nil {
		want = []SymbolReference{}
	}
	if got == nil {
		got = []SymbolReference{}
	}

	if len(got) != len(want) {
		t.Errorf("got %d refs, want %d", len(got), len(want))
		for _, r := range got {
			t.Logf("  got: %s (%s, inChange=%v)", r.Name, r.Kind, r.InChange)
		}
		return
	}

	for i := range want {
		if got[i].Name != want[i].Name {
			t.Errorf("ref[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		if got[i].Kind != want[i].Kind {
			t.Errorf("ref[%d].Kind = %q, want %q", i, got[i].Kind, want[i].Kind)
		}
		if got[i].InChange != want[i].InChange {
			t.Errorf("ref[%d].InChange = %v, want %v", i, got[i].InChange, want[i].InChange)
		}
	}
}

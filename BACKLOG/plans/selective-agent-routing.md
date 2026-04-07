# Selective Agent Routing with Cross-Category Context

## Context

Every PR currently dispatches all 7 agents with the full compressed diff. Three problems:

1. **Wasted tokens**: Agents see files they have no expertise on.
2. **Missing context**: When agents only see their relevant files, they lose cross-category dependencies.
3. **Scope guessing**: Agents guess whether issues are "changed" or "pre-existing" based on vibes rather than data.

## Goal

Build a shared symbol index as a pipeline-level resource. Use it for three purposes:
1. **Routing**: classify files, assemble per-agent diffs, inject cross-category references
2. **Scope hints**: tell agents which symbols are new/modified/pre-existing in this PR
3. **Verification**: post-dedup verification (existing verifier feature)

One index build, three consumers.

## Unified Pipeline

```
[Fetch PR] → [Compress diff (for agents)]
                    ↓
[Build symbol index from working tree (NOT from compressed diff)]
                    ↓
[Classify files] → [Split diff by file] → [Build change map]
                    ↓
              ReviewContext:
                []ClassifiedFile
                *Index (shared, immutable)
                *Resolver (shared)
                ChangeMap (new/modified/existing symbols)
                    ↓
[For each agent in parallel]:
    ├─ Filter files by role relevance
    ├─ Resolve cross-category refs via index (just signatures)
    ├─ Include scope hints from change map
    ├─ Build prompt: diff + cross-refs + scope hints
    └─ LLM call
                    ↓
[Collect] → [Dedup] → [Verify (same index + resolver)] → [Score] → [Report]
```

## Critical: Index vs Compression Independence

The diff gets compressed (strip lock files, vendor, generated code) before agents see it. The symbol index is built from the **raw working tree**, not the compressed diff. These are independent operations:

- **Compression** reduces what agents read (token savings)
- **Indexing** catalogs what the codebase contains (context lookup)

A generated file like `api.pb.go` is stripped from the diff (agents don't review it) but IS indexed (because application code imports its types). If we indexed only from the compressed diff, we'd miss symbols that cross-references need.

### Indexing Strategy

**v1: Full index.** Walk the entire repo (minus `node_modules/`, `vendor/`, `.git/`, etc). For a typical project (5k-20k files), this takes 1-3 seconds. Simple, guarantees no missed symbols.

**Future: Targeted index.** Parse the diff for imports, resolve the dependency graph, index only reachable files. Faster for large monorepos (100k+ files) where full indexing could take 10+ seconds. Requires language-specific import graph resolution (Go imports are straightforward, TypeScript has path aliases, barrel files, re-exports). Not v1 — flagged as future enhancement with `--index-mode=full|targeted`.

**Memory:** 10k symbols × ~100 bytes ≈ 1MB. Not a concern even for full index.

## Core Data Structures

### ClassifiedFile

```go
type ClassifiedFile struct {
    Path     string
    Category PRCategory
    Diff     string // compressed diff section for this file
}
```

Single source of truth per file. No parallel maps.

### ReviewContext

```go
type ReviewContext struct {
    Files     []ClassifiedFile
    Index     *index.Index       // nil if build failed
    Resolver  *resolve.Resolver  // nil if build failed
    ChangeMap *ChangeMap         // nil if build failed
}

// SymbolStatus returns the change status of a symbol, nil-safe.
// Returns SymbolExisting when ChangeMap is nil (graceful degradation).
func (rc *ReviewContext) SymbolStatus(file, name string) SymbolStatus {
    if rc.ChangeMap == nil {
        return SymbolExisting
    }
    return rc.ChangeMap.Status(file, name)
}
```

Passed explicitly through function arguments. Not stored as mutable Orchestrator state. Nil-safe helper eliminates nil-check ceremonies at call sites.

### ChangeMap

Keyed by `file:name` to avoid collision when the same symbol name exists in multiple files (e.g., `Init()` in `cmd/server.go` and `internal/db/db.go`).

```go
type SymbolStatus string
const (
    SymbolAdded    SymbolStatus = "new in this PR"
    SymbolModified SymbolStatus = "modified in this PR"
    SymbolExisting SymbolStatus = "pre-existing"
)

type SymbolKey struct {
    File string
    Name string
}

type ChangeMap struct {
    Modified map[SymbolKey]bool
    Added    map[SymbolKey]bool
}

func (cm *ChangeMap) Status(file, name string) SymbolStatus {
    key := SymbolKey{File: file, Name: name}
    if cm.Added[key]    { return SymbolAdded }
    if cm.Modified[key] { return SymbolModified }
    return SymbolExisting
}
```

Built by cross-referencing the diff's changed line ranges against the index's symbol locations.

## Design

### Per-File Classification

```go
func classifyFile(path string) PRCategory
```

Classification implemented as a `[]classifier` loop — adding a new pass is adding a slice element, not editing control flow:

```go
type classifier func(path, base, ext string) (PRCategory, bool)

var classifiers = []classifier{
    classifyByDirectory,  // pass 1: test/, docs/, .github/
    classifyByBasename,   // pass 2: Dockerfile, README
    classifyBySuffix,     // pass 3: _test.go, .test.ts, .spec.js
    classifyByExtension,  // pass 4: .md, .yml, .json
}

func classifyFile(path string) PRCategory {
    base := filepath.Base(path)
    ext := strings.ToLower(filepath.Ext(path))
    for _, c := range classifiers {
        if cat, ok := c(path, base, ext); ok {
            return cat
        }
    }
    return PRCategoryCode
}
```

### Role Relevance

```go
type Role struct {
    // ...existing fields...
    Relevance []PRCategory // PR categories this role sees; checked with SeeAll
    SeeAll    bool         // if true, sees all files regardless of Relevance
}
```

Zero value (`SeeAll: false`, `Relevance: nil`) → agent skipped. Safe default.

| Role | SeeAll | Relevance |
|------|--------|-----------|
| Know-It-All | `true` | — |
| Architect | `false` | `[code, config]` |
| Solver | `false` | `[code, tests]` |
| Editor | `false` | `[code, docs]` |
| Optimizer | `false` | `[code]` |
| Sentinel | `false` | `[code, config]` |
| Test Engineer | `false` | `[code, tests]` |

### Diff Assembly

```go
func FilterFilesForRole(role *Role, files []ClassifiedFile) []ClassifiedFile
func AssembleDiff(files []ClassifiedFile) string
```

### Diff Splitting

Single clean export from `internal/diff/`:

```go
// SplitToMap splits a unified diff into per-file sections, keyed by file path.
// Sections whose path cannot be extracted are logged and skipped.
func SplitToMap(rawDiff string) map[string]string
```

Combines the existing `splitDiffByFile` + `extractFilePath` into one export instead of exposing two building blocks the caller must assemble.

## Symbol Extraction: Monkey-Style Lexer

### Why a lexer, not regex

Regex (`\b[A-Za-z_]\w+\b`) matches every identifier-shaped string — locals, keywords, string contents, comments. The index filter helps but doesn't eliminate false matches on common names (`Error`, `Config`, `New`). A lexer is:

1. **More correct** — matches `ProcessBatch(` as a call, ignores `cfg` as a local
2. **More scalable** — adding new patterns (imports, generics) is extending token handling, not debugging regex edge cases
3. **More fun** — building a Thorsten Ball-style lexer in a real production tool is a great learning opportunity

### Package: `internal/lex/`

The lexer is a standalone, testable package with no dependencies on the index or agents. It tokenizes source lines and extracts symbol candidates.

**Language awareness:** The lexer unions Go and TypeScript keywords into one set (`func`, `type`, `var`, `const`, `function`, `class`, `interface`, `export`, `async`, `new`). These don't conflict — `func` doesn't appear in TS, `function`/`class` are valid but rare in Go. Documented as a v1 simplification; if future languages introduce keyword conflicts, add a language parameter.

**Unicode identifiers:** The lexer uses `byte`-level scanning (Monkey-style `ch byte`). This handles all Go identifiers (exported names must start with ASCII uppercase) and nearly all TypeScript identifiers in practice. Unicode identifiers in TS are a known v1 limitation — extremely rare in real code.

```go
// internal/lex/token.go

type Token struct {
    Type    TokenType
    Literal string
}

type TokenType int
const (
    TokenIdent TokenType = iota
    TokenLParen          // (
    TokenColon           // :
    TokenDot             // .
    TokenString          // entire string literal (contents skipped)
    TokenComment         // entire comment (contents skipped)
    TokenKeyword         // func, type, class, etc.
    TokenOther
    TokenEOL
)
```

```go
// internal/lex/lexer.go

// Lexer tokenizes a single line of source code (diff prefix already stripped).
// Skips string literal contents and comment contents.
// Based on Thorsten Ball's lexer design from "Writing an Interpreter in Go."
type Lexer struct {
    input   string
    pos     int
    readPos int
    ch      byte
}

func New(input string) *Lexer
func (l *Lexer) NextToken() Token
```

```go
// internal/lex/extract.go

// SymbolReference represents a symbol found in source text.
type SymbolReference struct {
    Name     string
    Kind     RefKind
    InChange bool // from a +/- line (not context)
}

type RefKind string
const (
    RefCall        RefKind = "call"        // Name( or pkg.Name(
    RefType        RefKind = "type"        // : Name, as Name
    RefDeclaration RefKind = "declaration" // func Name, type Name, class Name
)

// ExtractCandidates scans diff text for symbol references.
// Strips diff prefixes (+/-/space), skips hunk headers, tokenizes each line,
// and matches token patterns for calls, types, and declarations.
// Returns candidates WITHOUT index filtering — caller filters separately.
func ExtractCandidates(diffText string) []SymbolReference
```

**Token patterns for reference detection:**
- `IDENT LPAREN` → function call (`ProcessBatch(`)
- `IDENT DOT IDENT LPAREN` → qualified call (`config.Load(`)
- `COLON IDENT` → type annotation in TS (`: SomeType`)
- `KEYWORD_NEW IDENT` → constructor (`new Router(`)

**Token patterns for declaration detection:**
- `KEYWORD_FUNC IDENT` → Go function declaration
- `KEYWORD_TYPE IDENT` → Go type declaration
- `KEYWORD_FUNCTION IDENT` → TS/JS function declaration
- `KEYWORD_CLASS IDENT` → TS/JS class declaration
- `KEYWORD_INTERFACE IDENT` → TS interface declaration

### Filtering (separate concern)

```go
// internal/agents/crossref.go

// FilterByIndex keeps only candidates whose names exist in the index.
func FilterByIndex(candidates []lex.SymbolReference, idx *index.Index) []lex.SymbolReference
```

Clean separation: the lexer knows nothing about the index. The crossref package does the filtering. Each is independently testable.

## Cross-Category Context Injection

### When to inject

- Agent does NOT have `SeeAll` (Know-It-All doesn't need injection)
- Agent's filtered files include non-code categories (tests, config)
- Index is available

### What gets injected

| Agent sees | Context from | What |
|-----------|-------------|------|
| Test files | Code files | Function signatures being tested |
| Config files | Code files | Functions that read/consume the config |
| Doc files | — | Nothing |
| Code files | — | Nothing (self-contained) |

### Resolution

```go
// CrossReference is a resolved symbol for context injection.
type CrossReference struct {
    Symbol       index.Symbol
    Text         string // definition source text (signature + key lines)
    ReferencedFrom string // which diff file referenced this symbol
}

// resolveCrossReferences resolves symbol references from non-code files
// in the agent's diff. Returns structured data, not formatted strings.
func resolveCrossReferences(
    agentFiles []ClassifiedFile,
    rctx       *ReviewContext,
) []CrossReference
```

**Returns structured `[]CrossReference`, not formatted strings.** Formatting is a separate thin layer. Tests assert on structured fields, not string matching.

**Caps:** max 5 cross-references per agent, max 30 lines per definition. **Ranking:** prefer symbols from changed lines (`InChange: true`) over context lines. Among changed-line references, prefer calls over types (calls are more likely to have behavioral relevance).

### Formatting (separate from resolution)

```go
func formatCrossReferences(refs []CrossReference) string
```

Produces:
```
<cross-references>
Referenced from test files:
  handler.go:HandleRequest() (lines 10-25):
    func HandleRequest(w http.ResponseWriter, r *http.Request) { ... }
</cross-references>
```

## Change Map Construction

```go
func BuildChangeMap(files []ClassifiedFile, idx *index.Index) *ChangeMap
```

**Critical: the index is built from the working tree, which already includes the PR's changes.** A newly added function like `func NewHelper()` IS in the index — we indexed it when we walked the repo. So "isn't in the index" is never true for symbols in the current PR. We cannot use index presence to distinguish added from modified.

**The diff itself tells us.** A declaration line with a `+` prefix means the declaration is new. A declaration on a context line (space prefix) with changed body lines means the function was modified.

Algorithm:
1. For each classified file, parse the diff and classify each line as added (`+`), removed (`-`), or context (space). Collect changed line ranges (line numbers with `+`/`-`).
2. For `+` lines, run `lex.ExtractCandidates` looking for **declaration patterns** (`RefDeclaration`). If the declaration line itself is a `+` line → mark the symbol as **added** (the entire declaration is new in this PR).
3. Iterate `idx.SymbolsInFile(file)` and check if each symbol's `[StartLine, EndLine]` range overlaps any changed line range → mark as **modified**. This is more efficient than per-line `EnclosingScope` lookups, and the intent is clearer: "which symbols were touched?" not "what scope is this line in?"
4. Step 2 runs before step 3. If a symbol is already marked as added, step 3 skips it.

## Scope Hints in Agent Prompts

File-qualified to avoid ambiguity when the same name exists in multiple files:

```
<scope-context>
Symbols modified in this PR: HandleRequest (handler.go), ProcessBatch (worker.go)
Symbols new in this PR: ValidateInput (validator.go)
All other symbols in the codebase are pre-existing.

When reporting findings, use this to determine scope:
- "changed": issue is in code added or modified in this PR
- "existing": issue is in pre-existing code
- "codebase": broader architectural concern
</scope-context>
```

**Tradeoffs:**
- **Cost:** ~100-200 tokens per agent. Net savings if it prevents even one false positive.
- **Renamed symbols:** Old name "deleted" + new name "added." Technically correct.

## Prompt Structure

```go
// AgentPromptContext groups prompt-specific pieces to avoid
// 5 undifferentiated string parameters (misorder bug risk).
type AgentPromptContext struct {
    Diff       string
    CrossRefs  string
    ScopeHints string
}

func buildAgentPrompt(role *Role, pr *gh.PR, pctx AgentPromptContext) string
```

## Index Lifecycle in Review()

```go
func (o *Orchestrator) Review(ctx context.Context, pr *gh.PR) (*ReviewResult, error) {
    // Build symbol index ONCE from working tree (not compressed diff).
    var idx *index.Index
    var resolver *resolve.Resolver
    if o.opts.RepoRoot != "" {
        var err error
        idx, err = index.Build(ctx, o.opts.RepoRoot, o.opts.Languages)
        if err != nil {
            o.errLogf("Symbol index failed: %v (routing without cross-refs/scope)\n", err)
        } else {
            resolver = resolve.NewResolver(idx, o.opts.RepoRoot)
        }
    }

    // Classify + split (works without index).
    rctx := buildReviewContext(pr, idx, resolver)

    // Phase 1: Dispatch with per-agent diffs + context.
    dr, err := o.dispatchAgents(pr, rctx)

    // Phase 2: Dedup.
    result := o.collectAndSummarize(dr.Feedbacks)

    // Phase 3: Verify (reuses same index + resolver from rctx).
    if o.opts.Verify && rctx.Resolver != nil {
        // verifier uses rctx.Resolver — no separate index build
    }
}
```

**Index failure degrades gracefully:** classification + diff splitting + routing still work (pure Go, no index needed). Only cross-references, scope hints, and verification are skipped. Token savings from routing are preserved.

## Skipping Agents

- `FilterFilesForRole` returns 0 files AND `--roles` NOT set → skip, empty Feedback
- `FilterFilesForRole` returns 0 files AND `--roles` set → full diff (respect user's choice), log warning
- Logged: `"⏭️ Skipping Optimizer (no relevant files)"`

## User Feedback

```
📋 File routing:
   5 code files → all agents
   2 doc files → Know-It-All, Editor
   1 config file → Know-It-All, Architect, Sentinel
   ⏭️  Skipping Optimizer (no relevant files)
```

With `--verbose`, per-file classification + cross-ref resolution details.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/lex/token.go` | `Token`, `TokenType` constants |
| `internal/lex/lexer.go` | `Lexer` — Monkey-style tokenizer, comment/string skipping |
| `internal/lex/extract.go` | `ExtractCandidates` — pattern matching on token sequences |
| `internal/lex/lexer_test.go` | Tokenization tests |
| `internal/lex/extract_test.go` | Symbol extraction tests (including golden file multi-hunk diff) |
| `internal/agents/routing.go` | `PRCategory`, `ClassifiedFile`, `ReviewContext`, `classifyFile` (classifier chain), `FilterFilesForRole`, `AssembleDiff`, `buildReviewContext` |
| `internal/agents/crossref.go` | `CrossReference`, `FilterByIndex`, `resolveCrossReferences`, `formatCrossReferences` |
| `internal/agents/changemap.go` | `ChangeMap`, `SymbolKey`, `SymbolStatus`, `BuildChangeMap` |
| `internal/agents/routing_test.go` | Classification, filtering, assembly tests |
| `internal/agents/crossref_test.go` | Cross-ref resolution, FilterByIndex, pipeline integration test |
| `internal/agents/changemap_test.go` | Change map construction, added vs modified detection |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` + `SeeAll bool` to Role, populate all 7 |
| `internal/agents/orchestrator.go` | Update `Review()` to build index once, create `ReviewContext`, pass to `dispatchAgents`. Update `runAgent()` to accept `ReviewContext`. Update `buildAgentPrompt` to use `AgentPromptContext`. Wire verifier to use `ReviewContext.Resolver`. |
| `internal/diff/compress.go` | Add `SplitToMap()` export (combines split + path extraction) |
| `cmd/review.go` | Always detect repo root. Print routing summary. |

## Testing

**Classification** (table-driven):
```go
{"json in test dir", "test/fixtures/data.json", PRCategoryTests},
{"go test file", "handler_test.go", PRCategoryTests},
{"ts spec", "Component.spec.tsx", PRCategoryTests},
{"dockerfile", "Dockerfile", PRCategoryConfig},
{"go code", "main.go", PRCategoryCode},
```

**Lexer: diff prefix stripping edge cases** (table-driven):
```go
{"hunk header skipped", "@@ -10,5 +10,7 @@ func Foo()", wantRefs: []},
{"no-newline marker", `\ No newline at end of file`, wantRefs: []},
{"diff header", "--- a/handler.go", wantRefs: []},
{"empty + line", "+", wantRefs: []},
{"tab-indented call", "+\tresult := Process(x)", wantRefs: [{Name:"Process", Kind:RefCall}]},
```

**Lexer tokenization** (table-driven):
```go
{"function call", "result := ProcessBatch(data)", tokens: [IDENT, OTHER, IDENT, LPAREN, ...]},
{"string skipped", `name := "hello"`, tokens: [IDENT, OTHER, STRING]},
{"comment skipped", "// call ProcessBatch", tokens: [COMMENT]},
```

**Symbol extraction** (table-driven, no index dependency):
```go
{"call", "+\tresult := ProcessBatch(data)", wantRefs: [{Name:"ProcessBatch", Kind:RefCall, InChange:true}]},
{"qualified call", " \tcfg := config.Load()", wantRefs: [{Name:"Load", Kind:RefCall, InChange:false}]},
{"declaration", "+func NewHelper() {", wantRefs: [{Name:"NewHelper", Kind:RefDeclaration, InChange:true}]},
{"string ignored", `+name := "ProcessBatch"`, wantRefs: []},
{"keyword ignored", "+\tif err != nil {", wantRefs: []},
```

**Golden file test** for lexer: realistic multi-hunk diff with added/removed/context lines, comments, strings, multiple function calls. Asserts on full extraction output.

**FilterByIndex** (table-driven, no lexer dependency):
```go
{"keeps indexed symbol", candidates: ["ProcessBatch"], idx has ProcessBatch, want: ["ProcessBatch"]},
{"drops unknown symbol", candidates: ["localVar"], idx empty, want: []},
```

**Cross-references** (structured assertions):
```go
{"test refs code", testFiles, wantRefNames: ["HandleRequest"]},
{"config refs consumer", configFiles, wantRefNames: ["LoadConfig"]},
{"code-only: no refs", codeFiles, wantRefs: empty},
{"index nil: no refs", nilIndex, wantRefs: empty},
{"dedup across files", testFileA refs HandleRequest (context) + testFileB refs HandleRequest (+ line),
    wantRefs: [{Symbol: HandleRequest, ReferencedFrom: testFileB}]},  // prefer changed-line ref
```

**Cross-reference selection**: sort all candidates by `(InChange desc, Kind=RefCall first)`, then take top 5. Not "take 5 changed-line refs, fill remaining with context" — strict sort-then-cap.

**Change map** (structured assertions):
```go
{"modified function", diff with changed lines inside HandleRequest, wantModified: [{handler.go, HandleRequest}]},
{"new function", diff with +func NewHelper, wantAdded: [{helper.go, NewHelper}]},
{"unchanged function", context-only lines, wantModified: empty},
{"same name different files", Init in two files, both modified independently},
{"nested declaration", diff where +func innerHelper is inside modified HandleRequest,
    wantAdded: [{handler.go, innerHelper}], wantModified: [{handler.go, HandleRequest}]},
{"new file all added", diff with only + lines containing two func declarations,
    wantAdded: [{new.go, FuncA}, {new.go, FuncB}], wantModified: empty},
```

**Pipeline integration test** (ExtractCandidates → FilterByIndex → resolveCrossReferences with realistic diff):
- Multi-hunk diff with test file importing code functions
- Verify candidates extracted, filtered, resolved end-to-end
- Assert on structured CrossReference output, not strings

**End-to-end prompt assembly** (verifies sections snap together correctly):
```go
{"sentinel on mixed PR", role=Sentinel, files=[code+config], index available,
    wantPromptContains: ["<pr-diff>", "<cross-references>", "<scope-context>"]},
{"optimizer on code-only PR", role=Optimizer, files=[code], index available,
    wantPromptContains: ["<pr-diff>"],
    wantPromptNotContains: ["<cross-references>"]},
{"editor with nil index", role=Editor, files=[code+docs], index=nil,
    wantPromptContains: ["<pr-diff>"],
    wantPromptNotContains: ["<cross-references>", "<scope-context>"]},
```

**Integration:**
- Mixed PR → agents get correct subsets + cross-refs + scope hints
- Agent with 0 files skipped (no --roles)
- Agent with 0 files + --roles → full diff
- Index fails → routing works, cross-refs and scope hints empty
- Full pipeline: routing → dispatch → dedup → verify (shares index)
- ChangeMap key collision: Init in cmd/server.go and internal/db/db.go tracked separately

## Edge Cases

- **Index build fails**: routing + diff splitting still work. Cross-refs, scope hints, and verification empty. Token savings from routing preserved.
- **No cross-references found**: agent sees its files, no extra blocks.
- **Agent gets 0 files (no --roles)**: skip entirely.
- **Agent gets 0 files (--roles set)**: full diff, log warning.
- **All files are code**: every agent gets full diff, no cross-refs. Same as current.
- **Renamed symbol**: old name "deleted" + new name "added" in change map.
- **Zero-value Role**: `SeeAll: false`, `Relevance: nil` → skipped. Safe.
- **`_test.go` suffix**: handled in pass 3 (suffix), not pass 4 (extension).
- **Deleted file**: not in working tree → not in index → cross-references to deleted code resolve to nothing. Correct: the code no longer exists.
- **`--roles` override + scope hints**: scope hints are derived from the ChangeMap, which is built from all files (not the agent's filtered subset). So `--roles sentinel` on a docs PR still gets scope hints from the full diff. Data flow: `buildReviewContext` builds ChangeMap from all files, scope hints are pulled from ChangeMap in `buildAgentPrompt`, independent of which files the agent sees.
- **Generated file stripped from diff but in index**: correct — index is from working tree.
- **Symbol name collision**: `Init` in two files keyed as `{cmd/server.go, Init}` vs `{internal/db/db.go, Init}`.
- **Unparseable diff section**: logged and skipped by `SplitToMap`, not silently dropped.

## Future Enhancements

- **Targeted indexing**: `--index-mode=targeted` — parse diff imports, resolve dependency graph, index only reachable files. For large monorepos.
- **Deeper lexer patterns**: `import { Name }`, `IDENT.IDENT` field access, generic type params.
- **Change map scope correction**: use change map post-review to correct agent scope classifications in dedup.

## Estimation

- ~200 lines lexer package (`internal/lex/`)
- ~350 lines routing + crossref + changemap (`internal/agents/`)
- ~50 lines diff export + orchestrator wiring
- ~450-500 lines tests (including golden file multi-hunk diff test)
- Total: ~1050-1100 lines new + modified
- Modifies 4 existing files
- Creates 1 new package (`internal/lex/`), 5 new files in `internal/agents/`

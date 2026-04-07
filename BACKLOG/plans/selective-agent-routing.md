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
```

Passed explicitly through function arguments. Not stored as mutable Orchestrator state.

### ChangeMap

```go
// ChangeMap tracks which symbols were introduced, modified, or pre-existed.
type ChangeMap struct {
    // Modified: symbols that exist in the index AND have +/- lines at their location.
    Modified map[string]bool
    // Added: symbols that appear in + lines but NOT in the index (new in this PR).
    Added map[string]bool
    // The index itself represents pre-existing symbols.
}

// Status returns the change status of a symbol.
func (cm *ChangeMap) Status(name string) string {
    if cm.Added[name]   { return "new in this PR" }
    if cm.Modified[name] { return "modified in this PR" }
    return "pre-existing"
}
```

Built by cross-referencing the diff's changed line ranges against the index's symbol locations.

## Design

### Per-File Classification

```go
func classifyFile(path string) PRCategory
```

Four-pass precedence:
1. **Directory prefix**: `test/`, `docs/`, `.github/`, `ci/` → `strings.HasPrefix`
2. **Basename**: `Dockerfile`, `README`, `.gitignore` → exact/prefix match
3. **Suffix convention**: `_test.go`, `.test.ts`, `.spec.js` → `strings.HasSuffix`
4. **Extension**: `.md`, `.yml`, `.json`, `.go`, `.ts` → direct comparison

### Role Relevance

```go
type Role struct {
    // ...existing fields...
    Relevance []PRCategory // PR categories this role sees; nil checked with SeeAll
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

### Cross-Category Context: Symbol Extraction via Lexer

Regex-based identifier extraction is brittle — it matches every identifier-shaped string (locals, keywords, string contents, comments). A lightweight lexer approach is more precise.

#### Scoped Lexer (Monkey-style)

Based on Thorsten Ball's lexer design. We don't need expression parsing or AST nodes — just enough tokenization to answer: "given a line of source code, what symbols does it reference?"

Token types needed:
- `IDENT` — identifier (function name, type name, variable)
- `LPAREN` — `(`
- `COLON` — `:`
- `STRING` — skip string literal contents
- `COMMENT` — skip comment contents

Pattern matching on token sequences:
- `IDENT LPAREN` → function call (the IDENT is a symbol reference)
- `COLON IDENT` → type annotation in TypeScript (`: SomeType`)
- `IDENT DOT IDENT LPAREN` → qualified call (`pkg.Func(`)
- `new IDENT` → constructor (TypeScript)

Everything else (local variables, keywords, operators) is ignored.

```go
// internal/agents/lexer.go

type Token struct {
    Type    TokenType
    Literal string
}

type TokenType int
const (
    TokenIdent TokenType = iota
    TokenLParen
    TokenColon
    TokenDot
    TokenString  // entire string literal (skipped content)
    TokenComment // entire comment (skipped content)
    TokenOther   // everything else
    TokenEOL
)

// Tokenize scans a single clean source line (diff prefix already stripped)
// and returns tokens. Skips string literal contents and comment contents.
func Tokenize(line string) []Token

// ExtractSymbolRefs finds symbol references in tokenized diff lines.
// Looks for IDENT+LPAREN (calls), COLON+IDENT (types), etc.
// Only returns names that exist in the index.
func ExtractSymbolRefs(diffText string, idx *index.Index) []SymbolReference

type SymbolReference struct {
    Name     string
    Kind     RefKind  // call, type
    InChange bool     // from +/- line (not context)
}

type RefKind string
const (
    RefCall RefKind = "call"
    RefType RefKind = "type"
)
```

**Why not a full parser:** We're scanning diff lines, not complete files. Diffs have partial context, jump between hunks, and mix added/removed/context lines. A full AST parser needs complete source. A line-level lexer handles the fragmented nature of diffs.

**Why not regex:** `\b[A-Za-z_]\w+\b` matches `cfg`, `err`, `nil`, `string`, `for` — all noise. The lexer matches `ProcessBatch(` and knows it's a call, not a local variable. Precision matters because every false positive is wasted context tokens.

### Cross-Reference Resolution

```go
// CrossReference is a resolved symbol for context injection.
type CrossReference struct {
    Symbol index.Symbol
    Text   string // definition source text (just signature + key lines)
    Source string // which diff file referenced this
}

// resolveCrossReferences resolves symbol references from non-code files
// in the agent's diff. Returns structured data, not formatted strings.
func resolveCrossReferences(
    agentFiles []ClassifiedFile,
    rctx       *ReviewContext,
) []CrossReference
```

**Caps:** max 5 cross-references per agent, max 30 lines per definition. Ranking prefers symbols from changed lines (`InChange: true`).

**When to skip:** Agent has `SeeAll` (already sees everything), or agent only has code files (self-contained), or Index is nil.

### Change Map Construction

```go
// BuildChangeMap cross-references diff changed-line ranges against the index.
func BuildChangeMap(files []ClassifiedFile, idx *index.Index) *ChangeMap
```

Algorithm:
1. For each classified file, parse the diff to find changed line ranges (lines with `+`/`-`)
2. For each changed line range, check `idx.EnclosingScope(file, line)` — if a symbol contains changed lines, it's **modified**
3. For `+` lines that don't fall inside any indexed symbol, extract identifiers via the lexer — if they look like declarations and aren't in the index, they're **added** (new in this PR)

### Scope Hints in Agent Prompts

When the change map is available, include scope context:

```
<scope-context>
Symbols modified in this PR: HandleRequest, ProcessBatch
Symbols new in this PR: ValidateInput
All other symbols in the codebase are pre-existing.

When reporting findings, use this to determine scope:
- "changed": issue is in code that was added or modified in this PR
- "existing": issue is in code that existed before this PR
- "codebase": broader architectural concern
</scope-context>
```

**Tradeoffs of scope hints (Option A):**
- **Cost:** ~100-200 tokens per agent for the hint block. Net savings if it prevents even one false positive that would otherwise need verification.
- **Accuracy edge case:** Renamed symbols appear as old name "deleted" + new name "added." Technically correct but could confuse an agent. Acceptable for v1.
- **Benefit:** Agents make fewer scope classification mistakes → fewer false positives → less verifier work → net token savings.

### Prompt Structure

```go
func buildAgentPrompt(role *Role, pr *gh.PR, agentDiff, crossRefContext, scopeHints string) string
```

```
<pr-title>...</pr-title>
<pr-description>...</pr-description>

<pr-diff>
[per-agent filtered diff]
</pr-diff>

[if cross-references exist:]
<cross-references>
Referenced from test files:
  handler.go:HandleRequest() (lines 10-25):
    func HandleRequest(w http.ResponseWriter, r *http.Request) { ... }
</cross-references>

[if scope hints available:]
<scope-context>
Symbols modified in this PR: HandleRequest, ProcessBatch
Symbols new in this PR: ValidateInput
</scope-context>

Respond with a JSON object containing an array of findings...
```

### Index Lifecycle in Review()

```go
func (o *Orchestrator) Review(ctx context.Context, pr *gh.PR) (*ReviewResult, error) {
    // Build symbol index ONCE from working tree (not compressed diff).
    var idx *index.Index
    var resolver *resolve.Resolver
    if o.opts.RepoRoot != "" {
        var err error
        idx, err = index.Build(ctx, o.opts.RepoRoot, o.opts.Languages)
        if err != nil {
            o.errLogf("Symbol index failed: %v (routing without cross-refs)\n", err)
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

### Skipping Agents

- `FilterFilesForRole` returns 0 files AND `--roles` NOT set → skip, return empty Feedback
- `FilterFilesForRole` returns 0 files AND `--roles` set → use full diff, log warning
- Agent skipped → logged: `"⏭️ Skipping Optimizer (no relevant files)"`

### `--roles` Override Policy

`--roles` controls which agents run, not what they see. Specified agents still get routed diffs when files match. When no files match, they get the full diff (user explicitly asked for this agent).

### User Feedback

```
📋 File routing:
   5 code files → all agents
   2 doc files → Know-It-All, Editor
   1 config file → Know-It-All, Architect, Sentinel
   ⏭️  Skipping Optimizer (no relevant files)
```

With `--verbose`, per-file classification and cross-ref resolution details.

## Diff Splitting

Export from `internal/diff/compress.go`:

```go
func SplitByFile(rawDiff string) []string
func ExtractFilePath(section string) string
```

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | `PRCategory`, `ClassifiedFile`, `ReviewContext`, `classifyFile`, `FilterFilesForRole`, `AssembleDiff`, `buildReviewContext` |
| `internal/agents/crossref.go` | `CrossReference`, `SymbolReference`, `resolveCrossReferences`, `formatCrossReferences`, `BuildChangeMap`, `ChangeMap` |
| `internal/agents/lexer.go` | `Token`, `Tokenize`, `ExtractSymbolRefs` — Monkey-style lexer for diff line scanning |
| `internal/agents/routing_test.go` | Classification, filtering, assembly tests |
| `internal/agents/crossref_test.go` | Cross-reference resolution, change map tests |
| `internal/agents/lexer_test.go` | Tokenization, symbol extraction tests |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` + `SeeAll bool` to Role, populate all 7 |
| `internal/agents/orchestrator.go` | Update `Review()` to build index once, create `ReviewContext`, pass to `dispatchAgents`. Update `runAgent()` to accept `ReviewContext`. Update `buildAgentPrompt` signature. Wire verifier to use same index/resolver from `ReviewContext`. |
| `internal/diff/compress.go` | Export `SplitByFile()` and `ExtractFilePath()` |
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

**Lexer** (table-driven):
```go
{"function call", "result := ProcessBatch(data)", wantRefs: [{Name:"ProcessBatch", Kind:RefCall}]},
{"qualified call", "cfg := config.Load()", wantRefs: [{Name:"Load", Kind:RefCall}]},
{"type annotation", "var x SomeType", wantRefs: [{Name:"SomeType", Kind:RefType}]},
{"string contents ignored", `name := "ProcessBatch"`, wantRefs: []},
{"comment ignored", "// calls ProcessBatch", wantRefs: []},
{"keyword ignored", "if err != nil {", wantRefs: []},
```

**Cross-references** (structured assertions):
```go
{"test refs code", testFiles, wantRefNames: ["HandleRequest"]},
{"config refs consumer", configFiles, wantRefNames: ["LoadConfig"]},
{"code-only: no refs", codeFiles, wantRefs: empty},
{"index nil: no refs", nilIndex, wantRefs: empty},
```

**Change map:**
```go
{"modified function", diffWithChangedLines, idx, wantModified: ["HandleRequest"]},
{"new function", diffWithAddedFunc, idx, wantAdded: ["NewHelper"]},
{"unchanged function", contextOnlyDiff, idx, wantModified: empty},
```

**Integration:**
- Mixed PR → agents get correct subsets + cross-refs + scope hints
- Agent with 0 files skipped (no --roles)
- Agent with 0 files + --roles → full diff
- Index fails → routing works, cross-refs and scope hints empty
- Full pipeline: routing → dispatch → dedup → verify (shares index)

## Edge Cases

- **Index build fails**: routing + diff splitting still work. Cross-refs and scope hints empty. Token savings from routing preserved.
- **No cross-references found**: agent sees its files, no extra blocks. Strictly better than current.
- **Agent gets 0 files (no --roles)**: skip entirely.
- **Agent gets 0 files (--roles set)**: full diff, log warning.
- **All files are code**: every agent gets full diff, no cross-refs. Same as current.
- **Renamed symbol**: appears as "added" (new name) in change map. Technically correct.
- **Zero-value Role**: `SeeAll: false`, `Relevance: nil` → skipped. Safe.
- **`_test.go` suffix**: handled in pass 3 (suffix), not pass 4 (extension).
- **Generated file stripped from diff but in index**: correct — index is from working tree, not compressed diff.

## Future Enhancements

- **Targeted indexing**: `--index-mode=targeted` — parse diff imports, resolve dependency graph, index only reachable files. For large monorepos where full index is slow (>10s).
- **Deeper lexer patterns**: `import { Name }`, `IDENT.IDENT` field access, generic type parameters.
- **Change map for scope correction**: use change map post-review to correct agent scope classifications in dedup, not just as prompt hints.

## Estimation

- ~400-500 lines new code (routing.go + crossref.go + lexer.go)
- ~300 lines tests
- Modifies 4 existing files
- Reuses index + resolver from verifier (no new packages)

# Selective Agent Routing with Cross-Category Context

## Context

Every PR currently dispatches all 7 agents with the full compressed diff. Two problems:

1. **Wasted tokens**: Sentinel reviews README changes. Optimizer reviews CI config. Agents see files they have no expertise on.
2. **Missing context**: When agents only see their relevant files, they lose cross-category dependencies — test files reference code, config is consumed by code.

## Goal

Classify each file, route per-agent diffs (agents only see relevant files), and inject cross-category reference context via the symbol index. Built on top of the verifier's index/resolver infrastructure so the index is built once and shared across routing and verification.

## Unified Pipeline

```
[Fetch PR] → [Compress diff] → [Build symbol index (once, may fail)]
    ↓
[Classify each file + split diff] → build []ClassifiedFile
    ↓                                    (works regardless of index)
[For each agent in parallel]:
    ├─ Filter ClassifiedFiles by role relevance
    ├─ If index available: resolve cross-category context
    ├─ Build prompt: agent diff + cross-category context
    └─ LLM call
    ↓
[Collect] → [Dedup] → [Verify (reuses same index + resolver)] → [Score] → [Report]
```

## Core Data Structure

Instead of parallel maps (`fileDiffs map[string]string` + `classifications map[string]PRCategory`), use a single source of truth:

```go
// ClassifiedFile bundles a file's path, category, and diff section.
// Eliminates sync obligations between separate maps.
type ClassifiedFile struct {
    Path     string
    Category PRCategory
    Diff     string // compressed diff section for this file
}
```

This is passed explicitly through function arguments — not stored as mutable state on the Orchestrator.

### ReviewContext

```go
// ReviewContext holds per-review data built at the start of Review()
// and passed explicitly to each agent dispatch. No mutable struct fields.
type ReviewContext struct {
    Files    []ClassifiedFile
    Resolver *resolve.Resolver // nil if index build failed
    Index    *index.Index      // nil if index build failed
}
```

This replaces the plan's original approach of adding fields to `Orchestrator`. Data flows through arguments, not shared mutable state.

## Design

### Per-File Classification

```go
// internal/agents/routing.go

type PRCategory string

const (
    PRCategoryDocs   PRCategory = "docs"
    PRCategoryTests  PRCategory = "tests"
    PRCategoryConfig PRCategory = "config"
    PRCategoryCode   PRCategory = "code"
)

func (c PRCategory) Valid() bool { ... }

// classifyFile returns the category for a single file path.
// Four-pass precedence:
//   1. Directory prefix (test/, docs/, .github/)
//   2. Basename match (Dockerfile, README)
//   3. Suffix convention (_test.go, .test.ts, .spec.js)
//   4. Extension (.md, .yml, .json)
func classifyFile(path string) PRCategory
```

**Precedence: directory prefix > basename > suffix convention > extension.** The `_test.go` suffix is explicitly a third pass, not an extension match (`.go` would be code).

Pattern implementation: `strings.HasPrefix` for directories (no `filepath.Match` for `**`), `strings.HasPrefix`/exact match for basenames, `strings.HasSuffix` for test conventions, direct comparison for extensions.

### Role Relevance

**Problem with `nil` = "all categories":** The zero value of `[]PRCategory` is `nil`, meaning a forgotten `Relevance` field silently makes a new role see everything (the most expensive default). This is a sentinel value, not a useful zero value.

**Solution:** Explicit `SeeAll` field:

```go
type Role struct {
    // ...existing fields...
    Relevance []PRCategory // PR categories this role sees files for
    SeeAll    bool         // if true, sees all files regardless of Relevance
}
```

Zero value: `SeeAll: false, Relevance: nil` → sees nothing → agent skipped. Safe default. You must explicitly opt in to either `SeeAll: true` or a `Relevance` list.

| Role | SeeAll | Relevance |
|------|--------|-----------|
| Know-It-All | `true` | — |
| Architect | `false` | `[code, config]` |
| Solver | `false` | `[code, tests]` |
| Editor | `false` | `[code, docs]` |
| Optimizer | `false` | `[code]` |
| Sentinel | `false` | `[code, config]` |
| Test Engineer | `false` | `[code, tests]` |

**Change from v2:** Architect now sees `config` (infrastructure-as-code, docker-compose restructures are architectural decisions).

### Agent Diff Assembly

```go
// FilterFilesForRole returns the ClassifiedFiles relevant to a role.
func FilterFilesForRole(role *Role, files []ClassifiedFile) []ClassifiedFile {
    if role.SeeAll {
        return files
    }
    out := make([]ClassifiedFile, 0, len(files))
    for _, f := range files {
        if slices.Contains(role.Relevance, f.Category) {
            out = append(out, f)
        }
    }
    return out
}

// AssembleDiff joins the diff sections from classified files into one string.
func AssembleDiff(files []ClassifiedFile) string {
    var b strings.Builder
    for _, f := range files {
        b.WriteString(f.Diff)
    }
    return b.String()
}
```

### Cross-Category Context Injection

When an agent sees a subset of files, it may miss dependencies in other categories. The resolver provides the missing context.

**When to inject cross-references:**
- Agent does NOT have `SeeAll` (Know-It-All doesn't need injection)
- Agent's filtered files include non-code categories (tests, config)
- Resolver is available (index build succeeded)

**What gets injected:**

| Agent sees | Context from | What |
|-----------|-------------|------|
| Test files | Code files | Function signatures being tested |
| Config files | Code files | Functions that read/consume the config |
| Doc files | — | Nothing (docs agents don't need code context) |
| Code files | — | Nothing (self-contained) |

#### Extracting Symbols from Diff Text

This is non-trivial. Diff text has `+`/`-` prefixes, hunk headers (`@@`), and context lines. The approach:

```go
// extractDiffIdentifiers extracts symbol names from diff text.
// Strips diff prefixes (+/-/space) to get clean source lines,
// then scans for identifiers that exist in the index.
func extractDiffIdentifiers(diffText string, idx *index.Index) []string {
    var identifiers []string
    seen := make(map[string]bool)

    for _, line := range strings.Split(diffText, "\n") {
        // Skip hunk headers and file headers.
        if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "diff ") ||
           strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
            continue
        }

        // Strip diff prefix to get clean source line.
        clean := line
        if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
            clean = line[1:]
        }

        // Extract identifiers using the same regex as the resolver.
        matches := identRe.FindAllString(clean, -1)
        for _, name := range matches {
            if seen[name] || keywords[name] || len(name) < 2 {
                continue
            }
            // Only keep names that exist in the index.
            if syms := idx.Lookup(name); len(syms) > 0 {
                seen[name] = true
                identifiers = append(identifiers, name)
            }
        }
    }
    return identifiers
}
```

This reuses the `identRe` regex and `keywords` map from the existing resolver.

#### Ranking Cross-References

When there are more candidates than the cap, prefer symbols from **changed lines** (`+`/`-` prefix) over context lines (space prefix). Changed code is what the agent is reviewing.

```go
// rankIdentifiers sorts identifiers by relevance.
// Symbols from changed lines rank higher than symbols from context lines.
func rankIdentifiers(diffText string, identifiers []string, idx *index.Index) []index.Symbol
```

#### Resolving and Formatting

```go
// CrossReference is a resolved symbol reference for context injection.
type CrossReference struct {
    Symbol index.Symbol
    Text   string // source text of the definition
    Source string // which file's diff referenced this symbol
}

// resolveCrossReferences resolves symbol references from non-code files
// in the agent's diff, returning structured data (not formatted strings).
func resolveCrossReferences(
    agentFiles []ClassifiedFile,
    allFiles   []ClassifiedFile,
    resolver   *resolve.Resolver,
    idx        *index.Index,
) []CrossReference
```

**Returns structured data, not formatted strings** (Dave Cheney). The caller formats for the prompt. Tests assert on structured data, not string matching.

**Caps:** max 5 cross-references, max 30 lines per reference definition. Ranking prefers changed-line references.

#### Formatting for Prompt

```go
// formatCrossReferences formats resolved references for the prompt.
func formatCrossReferences(refs []CrossReference) string
```

Produces:
```
<cross-references>
Referenced from test files:
  handler.go:HandleRequest() (lines 10-25):
    func HandleRequest(w http.ResponseWriter, r *http.Request) {
        ...
    }
</cross-references>
```

### Prompt Structure (Updated)

```go
func buildAgentPrompt(role *Role, pr *gh.PR, agentDiff, crossRefContext string) string
```

Takes an explicit diff string and optional cross-ref context. If `crossRefContext` is empty, the `<cross-references>` block is omitted.

### Index Lifecycle in Review()

```go
func (o *Orchestrator) Review(ctx context.Context, pr *gh.PR) (*ReviewResult, error) {
    // Build symbol index ONCE — shared by routing + verifier.
    var idx *index.Index
    var resolver *resolve.Resolver
    if o.opts.RepoRoot != "" {
        var err error
        idx, err = index.Build(ctx, o.opts.RepoRoot, o.opts.Languages)
        if err != nil {
            o.errLogf("Symbol index build failed: %v (routing without cross-refs)\n", err)
            // idx and resolver stay nil — routing still works, just no cross-refs
        } else {
            o.logf("   📚 Symbol index: %d symbols indexed\n", idx.Size())
            resolver = resolve.NewResolver(idx, o.opts.RepoRoot)
        }
    }

    // Classify files and split diff — works regardless of index.
    reviewCtx := buildReviewContext(pr, resolver, idx)

    // Phase 1: Dispatch (each agent gets tailored diff + context).
    dr, err := o.dispatchAgents(pr, reviewCtx)
    // ...

    // Phase 3: Verify (reuses same resolver).
    if o.opts.Verify && resolver != nil {
        // ...
    }
}
```

**Key:** Classification and diff splitting work even if the index fails. Only cross-references are skipped. Token savings from routing are preserved.

### buildReviewContext

```go
func buildReviewContext(pr *gh.PR, resolver *resolve.Resolver, idx *index.Index) *ReviewContext {
    sections := diff.SplitByFile(pr.Diff)

    var files []ClassifiedFile
    for _, section := range sections {
        path := diff.ExtractFilePath(section)
        if path == "" {
            // Log warning: unrecognized diff section
            continue
        }
        files = append(files, ClassifiedFile{
            Path:     path,
            Category: classifyFile(path),
            Diff:     section,
        })
    }

    return &ReviewContext{
        Files:    files,
        Resolver: resolver,
        Index:    idx,
    }
}
```

Files not parseable from the diff are logged and skipped (not silently dropped).

### Skipping Agents

When `FilterFilesForRole` returns 0 files and `--roles` was NOT explicitly set:
- No LLM call
- Empty `Feedback` returned
- Logged: `"⏭️ Skipping Optimizer (no relevant files)"`

When `--roles` WAS explicitly set and an agent gets 0 relevant files:
- Send the full diff (user explicitly asked for this agent)
- Logged: `"⚠️ Sentinel: no code/config files detected, using full diff (--roles override)"`

### `--roles` Override Policy

`--roles` controls **which agents run**, not **what they see**. When `--roles` is set:
- Only the specified agents run (existing behavior)
- Those agents still get per-file routed diffs if files match their relevance
- If no files match, they get the full diff (respect the user's explicit choice)

### User Feedback

```
📋 File routing:
   5 code files → all agents
   2 doc files → Know-It-All, Editor
   1 config file → Know-It-All, Architect, Sentinel
   ⏭️  Skipping Optimizer (no relevant files)
   ⏭️  Skipping Test Engineer (no relevant files)
```

With `--verbose`, per-file classification:
```
      handler.go → code
      handler_test.go → tests
      README.md → docs
      .github/workflows/ci.yml → config
```

Verbose logging is internal — no need to export `classifyFile`.

## Diff Splitting

Export existing functions from `internal/diff/compress.go`:

```go
// SplitByFile splits a unified diff into per-file sections.
func SplitByFile(rawDiff string) []string

// ExtractFilePath extracts the file path from a diff section header.
func ExtractFilePath(section string) string
```

Renamed exports of existing `splitDiffByFile` and `extractFilePath`.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | `PRCategory`, `ClassifiedFile`, `ReviewContext`, `classifyFile`, `FilterFilesForRole`, `AssembleDiff`, pattern matching |
| `internal/agents/crossref.go` | `CrossReference`, `extractDiffIdentifiers`, `rankIdentifiers`, `resolveCrossReferences`, `formatCrossReferences` |
| `internal/agents/routing_test.go` | Classification, filtering, assembly tests |
| `internal/agents/crossref_test.go` | Identifier extraction, ranking, resolution, formatting tests |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` + `SeeAll bool` to Role, populate on all 7 roles |
| `internal/agents/orchestrator.go` | Update `Review()` to build `ReviewContext`, pass to `dispatchAgents`. Update `runAgent()` to accept `ReviewContext`, call `FilterFilesForRole` + `AssembleDiff` + `resolveCrossReferences`. Update `buildAgentPrompt` signature to `(role, pr, agentDiff, crossRefContext string)`. Add skip logic. |
| `internal/diff/compress.go` | Export `SplitByFile()` and `ExtractFilePath()` |
| `cmd/review.go` | Always detect repo root (needed for index). Print routing summary. |

## Testing

**Classification** (table-driven):
```go
{"json in test dir", "test/fixtures/data.json", PRCategoryTests},
{"go test file", "handler_test.go", PRCategoryTests},       // suffix pass
{"ts spec file", "Component.spec.tsx", PRCategoryTests},     // suffix pass
{"dockerfile", "Dockerfile", PRCategoryConfig},              // basename pass
{"github workflow", ".github/workflows/ci.yml", PRCategoryConfig}, // directory pass
{"go code", "main.go", PRCategoryCode},                      // extension pass (fallthrough)
```

**FilterFilesForRole**:
```go
{"architect sees code + config", RoleArchitect, mixedFiles, wantPaths: ["handler.go", "Dockerfile"]},
{"editor sees code + docs", RoleEditor, mixedFiles, wantPaths: ["handler.go", "README.md"]},
{"optimizer skipped on docs-only", RoleOptimizer, docsOnlyFiles, wantPaths: []},
{"know-it-all sees everything", RoleKnowItAll, mixedFiles, wantPaths: allPaths},
```

**extractDiffIdentifiers**:
```go
{"extracts function call from added line", "+\tresult := ProcessBatch(data)", wantContains: "ProcessBatch"},
{"skips keywords", "+\tif err != nil {", wantNotContains: "if"},
{"skips hunk headers", "@@ -10,5 +10,7 @@", wantEmpty: true},
{"strips diff prefix", "+\tcfg := LoadConfig()", wantContains: "LoadConfig"},
```

**resolveCrossReferences** (structured assertions):
```go
{"test file refs code", testFiles, wantRefNames: ["HandleRequest"]},
{"config file refs consumer", configFiles, wantRefNames: ["LoadConfig"]},
{"code-only: no refs needed", codeFiles, wantEmpty: true},
{"index nil: empty refs", noIndex, wantEmpty: true},
```

**Integration**:
- Mixed PR → each agent gets correct file subset + cross-refs
- Agent with 0 files skipped (no --roles)
- Agent with 0 files + --roles override → gets full diff
- Index build fails → routing works, cross-refs empty
- Full pipeline: routing → dispatch → dedup → verify (shares index)

## Edge Cases

- **Index build fails**: routing + diff splitting still work (pure Go). Only cross-references skipped. Token savings preserved.
- **No cross-references found**: agent sees its files, no `<cross-references>` block. Strictly better than current (less noise).
- **Agent gets 0 files (no --roles)**: skip entirely, return empty Feedback.
- **Agent gets 0 files (--roles set)**: use full diff, log warning.
- **All files are code**: every agent gets full diff, no cross-refs. Identical to current.
- **File in diff not parseable**: log warning, skip section (not silently dropped).
- **Zero-value Role (SeeAll: false, Relevance: nil)**: agent skipped. Safe default.
- **`_test.go` suffix**: handled in suffix pass (pass 3), not extension pass.

## Design Notes

**Why `ReviewContext` struct, not Orchestrator fields (Rob Pike):** Data flows through arguments, not shared mutable state. `runAgent` receives everything it needs explicitly. No temporal coupling between `Review()` setting fields and `runAgent()` reading them.

**Why `SeeAll` + `Relevance`, not nil sentinel (Thorsten Ball):** Zero value of `SeeAll: false, Relevance: nil` is the safe default (agent skipped). You must explicitly opt in. No accidental "see everything" from a forgotten field.

**Why structured `CrossReference`, not formatted string (Dave Cheney):** Separate data from formatting. Tests assert on structured fields. The prompt formatter is a thin layer on top.

**Why `[]ClassifiedFile`, not parallel maps (Bill Kennedy):** Single struct eliminates the sync obligation between separate maps keyed by the same string. No "file in diff but not in classifications" bug class.

## Estimation

- ~400-500 lines new code (routing.go + crossref.go)
- ~250 lines tests
- Modifies 4 existing files
- Reuses index + resolver from verifier (no new packages)

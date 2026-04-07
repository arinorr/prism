# Selective Agent Routing with Cross-Category Context

## Context

Every PR currently dispatches all 7 agents with the full compressed diff. Two problems:

1. **Wasted tokens**: Sentinel reviews README changes. Optimizer reviews CI config. Agents see files they have no expertise on.
2. **Missing context**: When agents only see their relevant files, they lose cross-category dependencies — test files reference code, config is consumed by code. This is the same "incomplete context" problem the verifier solves.

## Goal

Classify each file, route per-agent diffs (agents only see relevant files), and inject cross-category reference context via the symbol index. Built on top of the verifier's index/resolver infrastructure so the index is built once and shared across routing and verification.

## Unified Pipeline

```
[Fetch PR] → [Compress diff] → [Build symbol index (once)]
    ↓
[Classify each file] → [Split diff by file]
    ↓
[For each agent in parallel]:
    ├─ Assemble agent diff (relevant files only)
    ├─ Resolve cross-category context via index/resolver
    │   (test files get code signatures, config gets consumer functions)
    ├─ Build prompt: agent diff + cross-category context
    └─ LLM call
    ↓
[Collect] → [Dedup] → [Verify (reuses same index + resolver)] → [Score] → [Report]
```

The symbol index is built once at the top of `Review()`. Both the routing context injection and the verifier share it.

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
// Precedence: directory prefix > basename > extension.
func classifyFile(path string) PRCategory

// ClassifyFile is the exported version for verbose logging.
func ClassifyFile(path string) PRCategory { return classifyFile(path) }

// classifyFiles classifies all file paths and returns a map.
func classifyFiles(paths []string) map[string]PRCategory
```

Precedence: directory prefix (`test/`, `docs/`, `.github/`) > basename (`Dockerfile`, `README`) > extension (`.md`, `.yml`). `strings.HasPrefix` for directories, `filepath.Match` for single-level globs.

### Agent Diff Assembly

```go
// AssembleAgentDiff builds a diff containing only files relevant to the role.
// Returns the assembled diff and the list of included file paths.
func AssembleAgentDiff(
    role *Role,
    fileDiffs map[string]string,
    classifications map[string]PRCategory,
) (diff string, includedFiles []string)
```

A role with `nil` Relevance (Know-It-All) gets all files. A role with `Relevance: [code]` (Architect) only gets files classified as `code`. If 0 files match, the agent is skipped entirely.

### Cross-Category Context Injection

This is the key addition over the v2 plan. When an agent gets a subset of files, it may miss dependencies in other categories. The resolver (already built for the verifier) provides the missing context.

**What each category needs from other categories:**

| Agent sees | Might need context from | What to inject |
|-----------|------------------------|----------------|
| Test files | Code files | Function signatures being tested (enclosing scope text) |
| Config files | Code files | Functions that read/consume the config |
| Doc files | Nothing | Docs agents don't need code context |
| Code files | Nothing | Code agents get the full code diff already |

**How it works:**

```go
// resolveRoutingContext returns cross-category reference context
// for files outside the agent's direct diff.
func resolveRoutingContext(
    role *Role,
    includedFiles []string,
    allFileDiffs map[string]string,
    classifications map[string]PRCategory,
    resolver *resolve.Resolver,
) string
```

For each included file that's NOT code (tests, config), scan the diff text for symbol references (imports, function calls) and resolve them via the index. Return a formatted context block:

```
<cross-references>
Referenced from test files:
  handler.go:HandleRequest() (lines 10-25):
    func HandleRequest(w http.ResponseWriter, r *http.Request) {
        cfg := LoadConfig()
        ...
    }

Referenced from config files:
  cmd/server.go:LoadConfig() (lines 5-15):
    func LoadConfig() *Config {
        return &Config{Port: os.Getenv("PORT")}
    }
</cross-references>
```

**Caps**: max 5 cross-references per agent, max 30 lines per reference. This keeps the context injection small relative to the actual diff.

**When to skip**: If the agent only sees code files, no cross-references are needed (code is self-contained in this context). If the agent sees all files (Know-It-All), cross-references add nothing.

### Prompt Structure (Updated)

```go
func buildAgentPrompt(role *Role, pr *gh.PR, agentDiff, crossRefContext string) string {
    prompt := fmt.Sprintf(`Review the following pull request changes...

<pr-title>%s</pr-title>
<pr-description>%s</pr-description>
<pr-diff>%s</pr-diff>`, pr.Title, pr.Body, agentDiff)

    if crossRefContext != "" {
        prompt += fmt.Sprintf(`

The following code definitions are referenced by files in this diff but are not
part of your review scope. Use them for context only — do not review them.

%s`, crossRefContext)
    }

    prompt += `

Respond with a JSON object...`
    return prompt
}
```

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
            o.errLogf("Symbol index build failed: %v\n", err)
        } else {
            o.logf("   📚 Symbol index: %d symbols indexed\n", idx.Size())
            resolver = resolve.NewResolver(idx, o.opts.RepoRoot)
        }
    }

    // Classify files and split diff.
    classifications := classifyFiles(extractPaths(pr.Files))
    fileDiffs := splitDiffToMap(pr.Diff) // uses existing splitDiffByFile

    // Store for use in runAgent.
    o.fileDiffs = fileDiffs
    o.classifications = classifications
    o.resolver = resolver  // may be nil if index failed

    // Phase 1: Dispatch (each agent gets tailored diff + context).
    dr, err := o.dispatchAgents(pr)
    // ...

    // Phase 3: Verify (reuses same resolver).
    if o.opts.Verify && resolver != nil {
        // verifier already uses resolver — no changes needed
    }
}
```

### Skipping Agents

When `AssembleAgentDiff` returns 0 files:
- No LLM call is made
- An empty `Feedback` is returned
- Logged: `"⏭️  Skipping Sentinel (no relevant files)"`
- The agent still appears in `AgentUsages` with zero usage (for transparency)

### Role Relevance (unchanged)

| Role | Relevance | Sees |
|------|-----------|------|
| Know-It-All | `nil` (all) | All files |
| Architect | `[code]` | Code files + no cross-refs needed |
| Solver | `[code, tests]` | Code + test files |
| Editor | `[code, docs]` | Code + doc files |
| Optimizer | `[code]` | Code files + no cross-refs needed |
| Sentinel | `[code, config]` | Code + config files + code-side consumers |
| Test Engineer | `[code, tests]` | Code + test files |

### User Feedback

```
📋 File routing:
   5 code files → all agents
   2 doc files → Know-It-All, Editor
   1 config file → Know-It-All, Sentinel
   ⏭️  Skipping Optimizer (no relevant files)
   ⏭️  Skipping Test Engineer (no relevant files)
```

With `--verbose`, show per-file classification + cross-reference resolution.

## Diff Splitting

Export existing functions from `internal/diff/compress.go`:

```go
// SplitByFile splits a unified diff into per-file sections.
// Each section starts with "diff --git" and includes all hunks for that file.
func SplitByFile(rawDiff string) []string

// ExtractFilePath extracts the file path from a diff section header.
func ExtractFilePath(section string) string
```

These are renamed exports of the existing `splitDiffByFile` and `extractFilePath`.

Helper to build the map:

```go
// In orchestrator or routing package
func splitDiffToMap(rawDiff string) map[string]string {
    sections := diff.SplitByFile(rawDiff)
    m := make(map[string]string, len(sections))
    for _, s := range sections {
        path := diff.ExtractFilePath(s)
        if path != "" {
            m[path] = s
        }
    }
    return m
}
```

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | `PRCategory`, `classifyFile`, `classifyFiles`, `AssembleAgentDiff`, `resolveRoutingContext`, patterns |
| `internal/agents/routing_test.go` | Classification, assembly, context injection, skip tests |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` to Role, populate on all 7 roles |
| `internal/agents/orchestrator.go` | Add `fileDiffs`/`classifications`/`resolver` fields. Update `Review()` to build index once, classify, split. Update `runAgent()` to use per-agent diff + context. Update `buildAgentPrompt` signature. |
| `internal/diff/compress.go` | Export `SplitByFile()` and `ExtractFilePath()` |
| `cmd/review.go` | Always detect repo root (needed for index, not just verifier). Print routing summary. |

## Testing

**Classification** (table-driven):
```go
{"json in test dir", "test/fixtures/data.json", PRCategoryTests},
{"go test file", "handler_test.go", PRCategoryTests},
{"dockerfile", "Dockerfile", PRCategoryConfig},
{"go code", "main.go", PRCategoryCode},
```

**AssembleAgentDiff**:
```go
{"architect sees only code", RoleArchitect, mixedDiffs, wantFiles: ["handler.go"]},
{"editor sees code + docs", RoleEditor, mixedDiffs, wantFiles: ["handler.go", "README.md"]},
{"optimizer skipped on docs-only", RoleOptimizer, docsOnlyDiffs, wantFiles: []},
```

**Cross-category context**:
- Test file referencing `HandleRequest` → context includes `HandleRequest` signature
- Config file with no code references → empty context
- Code-only agent → no context injection
- Know-It-All (sees everything) → no context injection

**Integration**:
- Mixed PR → each agent gets correct subset + context
- Agent with 0 files skipped, empty feedback returned
- Full pipeline: routing → dispatch → dedup → verify (shares index)
- Index build failure → falls back to full diff (no routing, no verify)

## Edge Cases

- **Index build fails**: fall back to current behavior (full diff to all agents, no verification). Both features degrade gracefully together.
- **Resolver returns no cross-references**: agent just sees its files, no `<cross-references>` block. Fine — this is the current behavior for those files.
- **Agent gets 0 files**: skip entirely, return empty Feedback.
- **All files are code**: every agent gets full diff, no cross-refs needed. Identical to current behavior.
- **File in diff but not in pr.Files**: classify by path from the diff section header.
- **`--roles` explicit**: routing still happens (agents get tailored diffs) but no agents are skipped. User chose the agents, we just optimize what they see.

## Design Notes

**Why not let agents query the index**: LLMs can't query Go data structures. We'd have to pre-serialize relevant parts, which is exactly what `resolveRoutingContext` does. The agent receives pre-assembled context, not a lookup interface.

**Backward compatibility**: `pr.Diff` still contains the full compressed diff. Reports, suggestions, and non-agent code that reads `pr.Diff` works unchanged. Per-file routing is internal to the orchestrator.

**Single index build**: The index is built once in `Review()` and shared by both routing context injection and the verifier. No duplicate work.

**Cap strategy**: Cross-references are capped at 5 references, 30 lines each (~150 lines max added context per agent). This is small relative to typical diffs and bounded regardless of repo size.

## Estimation

- ~300 lines of new code (routing.go) + ~200 lines of tests
- Modifies 4 existing files
- Reuses index + resolver from verifier (no new packages)

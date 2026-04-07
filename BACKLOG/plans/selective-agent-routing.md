# Selective Agent Routing

## Context

Every PR currently dispatches all 7 agents regardless of content. A docs-only PR burns 7 LLM calls when 2 would suffice. This wastes tokens and money without improving review quality — Sentinel has nothing meaningful to say about a README change.

## Goal

Classify PRs by content type and only dispatch relevant agents. When the entire PR is homogeneous (all docs, all tests, all config), prune the agent list. Mixed PRs get the full squad.

## Design

### PR Classification

Classify based on **all files** in `pr.Files`. If every file matches a category, the PR is that category. If any file doesn't match, it's "code" and gets all agents.

| Category | File patterns | Agents |
|----------|--------------|--------|
| **docs** | `.md`, `.txt`, `.rst`, `.adoc`, `docs/**`, `README*`, `CHANGELOG*`, `LICENSE*`, `CONTRIBUTING*` | Know-It-All, Editor |
| **tests** | `*_test.go`, `*.test.ts`, `*.test.js`, `*.spec.ts`, `*.spec.js`, `test/**`, `__tests__/**`, `tests/**` | Test Engineer, Know-It-All, Solver |
| **config** | `.yml`, `.yaml`, `.json`, `.toml`, `.ini`, `.env*`, `Dockerfile*`, `docker-compose*`, `.github/**`, `ci/**`, `.gitignore`, `.eslintrc*`, `tsconfig*`, `go.mod`, `go.sum` | Know-It-All, Sentinel |
| **code** | anything else | All 7 |

### Classification Logic

New function in `internal/agents/routing.go`:

```go
type PRCategory string

const (
    PRCategoryDocs   PRCategory = "docs"
    PRCategoryTests  PRCategory = "tests"
    PRCategoryConfig PRCategory = "config"
    PRCategoryCode   PRCategory = "code"
)

// Valid returns true if the category is one of the known values.
// Consistent with the Risk.Valid() pattern used elsewhere in the codebase.
func (c PRCategory) Valid() bool {
    switch c {
    case PRCategoryDocs, PRCategoryTests, PRCategoryConfig, PRCategoryCode:
        return true
    }
    return false
}

// ClassifyPR determines the PR category from its file paths.
// Returns PRCategoryCode if any file doesn't match a specialized category.
// Accepts []string (just paths) rather than []gh.FileChange to keep
// routing decoupled from the GitHub client type and simplify testing.
func ClassifyPR(paths []string) PRCategory

// RouteAgents returns the subset of roles appropriate for the PR category.
// Returns all roles for PRCategoryCode.
func RouteAgents(roles []Role, category PRCategory) []Role
```

**Design note (Rob Pike):** `ClassifyPR` accepts `[]string` (file paths) not `[]gh.FileChange`. Classification only needs paths — accepting the full struct would couple routing to the GitHub client's type and force tests to construct `FileChange` structs for no reason. The caller extracts paths at the call site (3 lines).

Pattern matching uses the same approach as `diff/patterns.go` — `filepath.Match` for globs, prefix matching for directory patterns, basename matching for exact names.

### Where It Hooks In

In `cmd/review.go`, after `resolveRoles()` and `fetchPR()`, before creating the orchestrator:

```go
roles, err := resolveRoles(&merged)
// ...
pr, client, err := fetchPR(opts)
// ...

// Route agents based on PR content (unless --roles was explicitly set).
if opts.rolesFlag == "" {
    paths := make([]string, len(pr.Files))
    for i, f := range pr.Files {
        paths[i] = f.Path
    }
    category := agents.ClassifyPR(paths)
    routed := agents.RouteAgents(roles, category)
    if len(routed) < len(roles) {
        names := make([]string, len(routed))
        for i, r := range routed {
            names[i] = r.Name
        }
        fmt.Printf("   📋 %s PR detected — running %d/%d agents: %s (use --roles to override)\n",
            category, len(routed), len(roles), strings.Join(names, ", "))
    }
    roles = routed
}
```

Key: **skip routing when `--roles` is explicitly set**. The user's explicit choice always wins. The override check lives in `cmd/review.go` (CLI layer), not in `RouteAgents` — keeping the routing function a pure filter that doesn't know about CLI flags.

### Role-to-Category Mapping (Option A)

Add a `Relevance` field to `Role`:

```go
type Role struct {
    // ...existing fields...
    Relevance []PRCategory // PR categories this role is relevant for; empty = all
}
```

Roles with empty `Relevance` run for every category. This parallels the existing `Specialties` field (which maps roles to finding categories) — `Relevance` maps roles to PR categories. The zero value (`nil`) is the safest default: a new role without `Relevance` set runs everywhere.

**Why on the struct, not a separate map (Thorsten Ball):** The relationship "which PR types am I relevant for?" is an intrinsic property of a role, like `Specialties`. Putting it on the struct makes it discoverable — someone adding a new role sees the field and thinks about it. A separate map in `routing.go` could silently drift.

| Role | Relevance |
|------|-----------|
| Know-It-All | `nil` (all) |
| Architect | `[code]` |
| Solver | `[code, tests]` |
| Editor | `[code, docs]` |
| Optimizer | `[code]` |
| Sentinel | `[code, config]` |
| Test Engineer | `[code, tests]` |

### RouteAgents Implementation

```go
func RouteAgents(roles []Role, category PRCategory) []Role {
    if category == PRCategoryCode {
        return roles
    }
    out := make([]Role, 0, len(roles)) // pre-allocate to max possible size
    for _, r := range roles {
        if len(r.Relevance) == 0 || slices.Contains(r.Relevance, category) {
            out = append(out, r)
        }
    }
    return out
}
```

**Design note (Bill Kennedy):** Pre-allocate `out` to `len(roles)` capacity. For 7 roles this is trivial but establishes the right habit — you know the upper bound.

### User Feedback

Print the routing decision before dispatch:

```
📋 docs PR detected — running 2/7 agents: Know-It-All, Editor (use --roles to override)
```

For code PRs (full squad), print nothing — that's the default and needs no explanation.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | `PRCategory`, `ClassifyPR`, `RouteAgents`, file pattern matching |
| `internal/agents/routing_test.go` | Classification + routing tests (table-driven) |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` to Role, populate on all 7 roles |
| `cmd/review.go` | Extract paths from `pr.Files`, call `ClassifyPR` + `RouteAgents`, print routing message |
| `cmd/review.go` `printEstimate()` | Estimate should use the routed role count, not all roles |

## Testing

Table-driven tests for `ClassifyPR` — the classification matrix maps directly to test table rows:

```go
tests := []struct {
    name  string
    paths []string
    want  PRCategory
}{
    {"docs only",   []string{"README.md", "docs/guide.md"}, PRCategoryDocs},
    {"tests only",  []string{"handler_test.go", "test/util_test.go"}, PRCategoryTests},
    {"config only", []string{".github/workflows/ci.yml", "Dockerfile"}, PRCategoryConfig},
    {"mixed",       []string{"handler.go", "README.md"}, PRCategoryCode},
    {"empty",       []string{}, PRCategoryCode},
}
```

Additional tests:
- `RouteAgents` for each category returns correct subset
- `RouteAgents` for code returns all roles unchanged
- `RouteAgents` with role that has `nil` Relevance always included
- Edge cases: `.github/workflows/ci.yml` is config, `test/fixtures/data.json` is tests
- `PRCategory.Valid()` returns true for known, false for unknown
- `--roles` override bypasses routing (CLI test in `cmd/`)
- Integration: full pipeline with docs-only PR runs fewer agents

## Edge Cases

- **Deleted files**: a PR that only deletes `.go` files — file extensions still classify as code
- **Renamed files**: `renamed` status, path still has extension — works fine
- **Mixed test + code**: `handler.go` + `handler_test.go` — not all-tests, classified as code, full squad
- **Lock files**: `go.sum` is config, but diff compression already strips it — if all remaining files are config after stripping, classify as config
- **No files**: empty file list — default to code (full squad)

## Estimation

- ~200 lines of new code + tests
- No new dependencies
- Token savings: 50-70% on docs/config PRs, 30-40% on test-only PRs

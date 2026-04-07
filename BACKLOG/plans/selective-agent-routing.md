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

### Pattern Matching Precedence

When a file matches multiple categories (e.g., `test/fixtures/data.json` matches both `test/**` and `.json`), **directory patterns take precedence over extension patterns**. A file's location tells you its purpose more than its extension — a JSON file inside a test directory is a test fixture, not config.

Matching order for each file:
1. **Directory prefix** — `strings.HasPrefix` for patterns like `test/**`, `docs/**`, `.github/**`
2. **Basename match** — exact names like `README*`, `Dockerfile*`, `Makefile`
3. **Extension match** — `.md`, `.yml`, `.json`, etc.

First match wins. If a file matches no specialized category, the entire PR is classified as "code."

### Pattern Implementation: No `filepath.Match` for `**`

`filepath.Match` does not support `**` (recursive glob) — it only matches within a single path segment. All directory patterns must use `strings.HasPrefix`:

```go
// WRONG: filepath.Match doesn't recurse
filepath.Match("docs/**", "docs/deep/nested/file.md") // false!

// RIGHT: prefix check for directory patterns
strings.HasPrefix(path, "docs/")     // true
strings.HasPrefix(path, "test/")     // true
strings.HasPrefix(path, ".github/")  // true
```

`filepath.Match` is only used for single-level globs like `*.test.ts` and basename patterns like `Dockerfile*`.

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

// classifyFile returns the category for a single file path.
// Checks directory prefix first, then basename, then extension.
func classifyFile(path string) PRCategory

// RouteAgents returns the subset of roles appropriate for the PR category.
// Returns all roles for PRCategoryCode.
func RouteAgents(roles []Role, category PRCategory) []Role
```

**Design note (Rob Pike):** `ClassifyPR` accepts `[]string` (file paths) not `[]gh.FileChange`. Classification only needs paths — accepting the full struct would couple routing to the GitHub client's type and force tests to construct `FileChange` structs for no reason. The caller extracts paths at the call site (3 lines).

### ClassifyPR Algorithm

```go
func ClassifyPR(paths []string) PRCategory {
    if len(paths) == 0 {
        return PRCategoryCode
    }

    category := classifyFile(paths[0])
    for _, p := range paths[1:] {
        if classifyFile(p) != category {
            return PRCategoryCode // mixed = full squad
        }
    }
    return category
}
```

All files must agree on the same category. If any file disagrees, it's code.

### pr.Files vs Compressed Diff

`ClassifyPR` runs on `pr.Files` which comes from the GitHub API (`gh pr view --json files`). This is the full list of files the PR touches, **not** the compressed diff. Compression only affects what agents *see*, not what files the PR *touches*. So:

- A PR touching only `go.sum` → `pr.Files = [{Path: "go.sum"}]` → classified as config → runs Know-It-All + Sentinel. The diff those agents see might be stripped by compression, but that's the compression system's problem, not routing's.
- A PR touching `main.go` + `go.sum` → mixed → code → full squad. Even though `go.sum` is stripped from the diff, `main.go` drives the classification.

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
    if opts.verbose {
        for _, p := range paths {
            fmt.Printf("      %s → %s\n", p, agents.ClassifyFile(p))
        }
    }
    roles = routed
}
```

Key: **skip routing when `--roles` is explicitly set**. The user's explicit choice always wins. The override check lives in `cmd/review.go` (CLI layer), not in `RouteAgents` — keeping the routing function a pure filter that doesn't know about CLI flags.

**Verbose logging** shows per-file classification so users can debug routing decisions when they disagree.

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

**Scaling note:** If PR categories grow past ~6, the per-role `Relevance` lists become noisy and a centralized map in `routing.go` may be cleaner. For 4 categories and 7 roles, the struct field is the right call.

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

**Design note (Bill Kennedy):** Pre-allocate `out` to `len(roles)` capacity.

### printEstimate Update

`printEstimate` currently receives all roles. After routing, it should receive the routed subset:

```go
// Before: printEstimate(len(compressed), roles)  — all 7
// After:  printEstimate(len(compressed), roles)   — routed subset (2-7)
```

This happens naturally since `roles` is reassigned before `printEstimate` is called. No code change needed in `printEstimate` itself — just verify the call site passes the routed roles.

### User Feedback

Print the routing decision before dispatch:

```
📋 docs PR detected — running 2/7 agents: Know-It-All, Editor (use --roles to override)
```

For code PRs (full squad), print nothing — that's the default and needs no explanation.

With `--verbose`, show per-file classification:

```
📋 docs PR detected — running 2/7 agents: Know-It-All, Editor (use --roles to override)
      README.md → docs
      docs/guide.md → docs
```

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | `PRCategory`, `ClassifyPR`, `classifyFile`, `RouteAgents`, pattern matching |
| `internal/agents/routing_test.go` | Classification + routing tests (table-driven) |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` to Role, populate on all 7 roles |
| `cmd/review.go` | Extract paths from `pr.Files`, call `ClassifyPR` + `RouteAgents`, print routing message + verbose per-file log |

## Testing

Table-driven tests for `ClassifyPR` — the classification matrix maps directly to test table rows:

```go
tests := []struct {
    name  string
    paths []string
    want  PRCategory
}{
    {"docs only",       []string{"README.md", "docs/guide.md"}, PRCategoryDocs},
    {"tests only",      []string{"handler_test.go", "test/util_test.go"}, PRCategoryTests},
    {"config only",     []string{".github/workflows/ci.yml", "Dockerfile"}, PRCategoryConfig},
    {"code",            []string{"handler.go", "main.go"}, PRCategoryCode},
    {"mixed doc+code",  []string{"handler.go", "README.md"}, PRCategoryCode},
    {"mixed test+code", []string{"handler.go", "handler_test.go"}, PRCategoryCode},
    {"empty",           []string{}, PRCategoryCode},
}
```

Additional tests:
- `classifyFile` precedence: `test/fixtures/data.json` → tests (directory wins over extension)
- `classifyFile`: `scripts/deploy.sh` alongside `.github/workflows/ci.yml` → mixed → code
- `RouteAgents` for each category returns correct subset
- `RouteAgents` for code returns all roles unchanged
- `RouteAgents` with role that has `nil` Relevance always included
- `PRCategory.Valid()` returns true for known, false for unknown
- `--roles` override bypasses routing (CLI test in `cmd/`)

## Edge Cases

- **Deleted files**: a PR that only deletes `.go` files — file extensions still classify as code
- **Renamed files**: `renamed` status, path still has extension — works fine
- **Mixed test + code**: `handler.go` + `handler_test.go` — not all-tests, classified as code, full squad
- **Lock files**: `go.sum` is config. Classification runs on `pr.Files` (GitHub API), not the compressed diff. A go.sum-only PR classifies as config correctly.
- **No files**: empty file list — default to code (full squad)
- **Directory precedence**: `test/fixtures/data.json` → tests (directory prefix wins over `.json` extension)
- **Shell scripts in CI PRs**: `scripts/deploy.sh` + `.github/workflows/ci.yml` → `.sh` doesn't match config → mixed → code
- **Makefile / Justfile**: no specialized pattern → code. Fine for v1.

## Future Categories (Out of Scope)

These patterns are worth recognizing eventually but are not v1:

- **Vendored / generated code**: `vendor/**`, `generated/**`, `*.pb.go`, `*_gen.go` — code by extension but reviewing is usually noise
- **Migration files**: `migrations/*.sql`, `db/migrate/**` — only Sentinel and maybe Architect care
- **CI-specific**: `Makefile`, `Justfile`, `scripts/*.sh` — could be a `ci` category alongside `config`

## Design Notes

**Path-based vs content-based classification**: Classification uses file paths, not diff content. This means a PR with a heavily compressed code diff alongside intact test files is still "mixed" (classified as code). This is the right trade for v1 — path classification is instant, deterministic, and correct for the common cases. Content-based classification would require reading diffs and is a different feature.

## Estimation

- ~200 lines of new code + tests
- No new dependencies
- Token savings: 50-70% on docs/config PRs, 30-40% on test-only PRs

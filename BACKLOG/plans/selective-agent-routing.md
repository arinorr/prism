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
    CategoryDocs   PRCategory = "docs"
    CategoryTests  PRCategory = "tests"
    CategoryConfig PRCategory = "config"
    CategoryCode   PRCategory = "code"
)

// ClassifyPR determines the PR category from its file list.
// Returns "code" if any file doesn't match a specialized category.
func ClassifyPR(files []gh.FileChange) PRCategory

// RouteAgents returns the subset of roles appropriate for the PR category.
// Returns all roles for "code" category.
func RouteAgents(roles []Role, category PRCategory) []Role
```

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
    category := agents.ClassifyPR(pr.Files)
    routed := agents.RouteAgents(roles, category)
    if len(routed) < len(roles) {
        fmt.Printf("   📋 %s PR detected — running %d/%d agents (use --roles to override)\n",
            category, len(routed), len(roles))
    }
    roles = routed
}
```

Key: **skip routing when `--roles` is explicitly set**. The user's explicit choice always wins.

### Role-to-Category Mapping

Add a field to `Role`:

```go
type Role struct {
    // ...existing fields...
    Relevance []PRCategory // PR categories this role is relevant for; empty = all
}
```

Roles with empty `Relevance` run for every category. This keeps the mapping on the Role itself rather than in a separate lookup table.

| Role | Relevance |
|------|-----------|
| Know-It-All | `[]` (all) |
| Architect | `[code]` |
| Solver | `[code, tests]` |
| Editor | `[code, docs]` |
| Optimizer | `[code]` |
| Sentinel | `[code, config]` |
| Test Engineer | `[code, tests]` |

### User Feedback

Print the routing decision before dispatch:

```
📋 docs PR detected — running 2/7 agents: Know-It-All, Editor (use --roles to override)
```

For code PRs (full squad), print nothing — that's the default and needs no explanation.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/routing.go` | ClassifyPR, RouteAgents, pattern matching |
| `internal/agents/routing_test.go` | Classification + routing tests |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/roles.go` | Add `Relevance []PRCategory` to Role, populate on all 7 roles |
| `cmd/review.go` | Call ClassifyPR + RouteAgents after fetchPR, print routing message |
| `cmd/review.go` `printEstimate()` | Estimate should use the routed role count, not all roles |

## Testing

- Unit: classify docs-only, tests-only, config-only, mixed, empty PR
- Unit: route agents for each category, verify correct subset
- Unit: edge cases — `.github/workflows/ci.yml` is config, `test/fixtures/data.json` is tests
- Unit: `--roles` override bypasses routing
- Integration: full pipeline with docs-only PR runs fewer agents

## Edge Cases

- **Deleted files**: a PR that only deletes `.go` files — file extensions still classify as code
- **Renamed files**: `renamed` status, path still has extension — works fine
- **Mixed test + code**: `handler.go` + `handler_test.go` — not all-tests, classified as code, full squad
- **Lock files**: `go.sum` is config, but diff compression already strips it — if all remaining files are config after stripping, classify as config
- **No files**: empty file list — default to code (full squad)

## Estimation

- ~150 lines of new code + tests
- No new dependencies
- Token savings: 50-70% on docs/config PRs, 30-40% on test-only PRs

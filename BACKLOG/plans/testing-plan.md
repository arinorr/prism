# Testing Plan: Routing + Verification

## Part 1: Real PR Before/After Testing

### Goal

Run Prism on real PRs across multiple repos and languages, comparing results with and without routing + verification. Measure: token savings, false positive reduction, routing accuracy, and any regressions.

### Test Matrix

| # | Repo | PR | Language | Type | What It Tests |
|---|------|-----|----------|------|--------------|
| 1 | arinorr/prism | #23 | Go | Code-only (3 files: cmd, agents, dedup) | Baseline: all-code PR, routing should send all files to all agents |
| 2 | arinorr/prism | #14 | Markdown + .tape | Mixed (docs + unknown) | Routing: README is docs, demo.tape is code → mixed → full squad |
| 3 | arinorr/prism | #22 | Go | Code (15 files: agents, config, report) | Large code PR, verifier should catch false positives in score refactoring |
| 4 | arinorr/prism | #18 | Go | Code + tests (10 files) | Mixed code+tests, routing should send tests to Test Engineer + Solver |
| 5 | arinorr/ari-cloud | #13 | TypeScript | Code (27 files: API + client) | TS monorepo, tests cross-refs between API contracts and implementations |
| 6 | arinorr/ari-cloud | #9 | TypeScript + .md | Mixed code + test + config | Has test files (`__tests__/`), route files, middleware — routing should split correctly |
| 7 | arinorr/whetstone | #1 | Go + YAML + Makefile + .md | Mixed (CI, docs, code, tests) | Best routing test: has .github/ci.yml, README, Makefile, Go code, test files |
| 8 | arinorr/whetstone | #3 | Go | Code + tests (20 files) | Go code with tests, verifier should use index to check cross-refs |

### For Each PR, Capture

**Baseline run** (current main behavior, no routing):
```bash
prism review <PR> --format json --stdout > baseline.json 2> baseline.log
```

**Routed + verified run** (new branch):
```bash
prism review <PR> --verify --format json --stdout > routed.json 2> routed.log
```

**Compare:**

| Metric | How to Measure |
|--------|---------------|
| Agent calls | Count LLM calls in log (baseline: always 7, routed: ≤ 7) |
| Agents skipped | Count "⏭️ Skipping" lines in routed log |
| Files per agent | Count from "reviewing (N files)" in routed log |
| Token usage | Compare total input tokens (JSON: usage.input_tokens) |
| Cost | Compare total cost (JSON: usage.cost_usd) |
| Finding count | Compare len(deduped_findings) |
| False positives dismissed | Count dismissed_count in routed JSON |
| Findings downgraded | Count downgraded_count in routed JSON |
| Routing classification | Verify per-file categories are correct (--verbose output) |
| Cross-refs injected | Check verbose log for cross-reference resolution |
| Scope hints | Check if modified/added symbols listed correctly |

### Success Criteria

| Metric | Target |
|--------|--------|
| Token savings (mixed PRs) | 10-30% reduction in input tokens |
| Token savings (homogeneous non-code PRs) | 50-70% reduction |
| False positive dismissal rate | > 50% of known false positives dismissed by verifier |
| False negative rate | 0% — real issues never dismissed |
| Routing accuracy | 100% correct file classification on test matrix |
| No regressions | Finding quality equal or better than baseline |

### Priority Order

Run PRs 7, 6, 4 first — these have the best mix of file types for testing routing. Then PRs 5, 3 for verifier testing. Then the rest for coverage.

---

## Part 2: Unit + Integration Test Gaps

68 untested code paths identified across the implementation. Grouped by priority.

### Priority 1: No Test Coverage At All

These functions/modules have zero test coverage and need tests immediately.

**crossref.go — entire module untested:**
- `ResolveCrossReferences` with non-code files referencing code symbols
- `ResolveCrossReferences` when rctx is nil → returns nil
- `ResolveCrossReferences` when agent only has code files → returns nil (no cross-refs needed)
- `FilterByIndex` with empty candidates → returns empty
- `FilterByIndex` filtering out unknown names
- `FormatCrossReferences` with empty/nil refs → returns empty string
- `FormatCrossReferences` produces valid `<cross-references>` XML block
- Cross-ref dedup: same symbol from two files, prefer InChange=true ref
- Cross-ref ranking: changed-line refs before context, calls before types
- Cross-ref cap: > 5 candidates capped to 5
- Cross-ref truncation: definition > 30 lines truncated with marker
- Resolution failure: `Resolver.Resolve` fails → candidate skipped gracefully

**changemap.go — helper functions untested:**
- `parseChangedLines` with multi-hunk diff (tracks line numbers across hunks)
- `parseChangedLines` with removed lines (don't increment newLine counter)
- `parseChangedLines` with empty lines between hunks
- `parseHunkNewStart` with various formats (`@@ -10,5 +15,7 @@`, `@@ -10 +15 @@`)
- `parseHunkNewStart` with malformed hunk (no `+`)
- `rangeOverlapsChanges` with overlapping/non-overlapping/empty ranges
- `FormatScopeHints` with nil ChangeMap → empty string
- `FormatScopeHints` with both empty Added and Modified → empty string
- `FormatScopeHints` output format correctness

**SplitToMap — new export untested:**
- `SplitToMap` with valid multi-file diff
- `SplitToMap` with section missing file path → skipped
- `SplitToMap` with malformed diff (no "diff --git" marker)
- `SplitToMap` with empty input

### Priority 2: Edge Cases in Tested Code

These functions have tests but miss important edge cases.

**Lexer edge cases:**
- Unclosed string literal → consumes to end of input
- Unclosed template literal → consumes to end of input
- Unclosed block comment → consumes to end of input
- Escaped backtick in template literal
- Consecutive escapes in string (`"foo\\\\bar"`)

**Extract edge cases:**
- `stripDiffPrefix` on empty string
- `stripDiffPrefix` on line without +/-/space prefix
- `matchPatterns` with token at boundary (COLON as last token before EOL)
- `shouldSkipDiffLine` with `old mode`, `new mode`, `index` lines

**Routing edge cases:**
- `BuildReviewContext` with nil index and nil resolver
- `BuildReviewContext` with files in pr.Files but not in diff
- `RoutingSummary` with empty roles
- `RoutingSummary` with zero files in all categories
- `ReviewContext.SymbolStatus` when ReviewContext itself is nil

**Orchestrator edge cases:**
- Agent skipped when routing returns 0 files (no --roles)
- Agent gets full diff when --roles set but 0 files match
- `buildAgentPrompt` with empty CrossRefs and ScopeHints (no extra blocks)
- `buildAgentPrompt` with populated CrossRefs and ScopeHints (all blocks present)

### Priority 3: Integration Tests

- **Pipeline integration:** Mixed PR → routing splits files → each agent gets correct subset → cross-refs injected → scope hints present → dedup → verify (if enabled) → score
- **Routing + skip integration:** PR with only .md files → only SeeAll agents run, rest skipped
- **Index failure integration:** Index build fails → routing works (full diff) → verify skipped → results still produced
- **Cross-ref pipeline:** Extract candidates → filter by index → resolve → format → appears in agent prompt

### Test Files to Create/Modify

| File | Tests to Add |
|------|-------------|
| `internal/agents/crossref_test.go` (NEW) | ~15 tests covering ResolveCrossReferences, FilterByIndex, FormatCrossReferences, dedup, ranking, cap, truncation |
| `internal/agents/changemap_test.go` | ~10 tests: parseChangedLines, parseHunkNewStart, rangeOverlapsChanges, FormatScopeHints edge cases |
| `internal/diff/compress_test.go` | ~4 tests: SplitToMap valid/malformed/empty/missing-path |
| `internal/lex/lexer_test.go` | ~5 tests: unclosed strings/comments/templates, consecutive escapes |
| `internal/lex/extract_test.go` | ~4 tests: stripDiffPrefix edge cases, shouldSkipDiffLine rare markers |
| `internal/agents/routing_test.go` | ~6 tests: BuildReviewContext, RoutingSummary, ReviewContext.SymbolStatus nil-safety |
| `internal/agents/orchestrator_test.go` | ~4 tests: agent skip/fallback, buildAgentPrompt with/without context blocks |

**Estimated: ~48 new test cases across 7 files.**

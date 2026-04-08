# Prism Backlog

Pre-release work items. Each links to a detailed plan.

## Active

1. [Prompt Cache Optimization](plans/prompt-cache-optimization.md) — restructure prompts for max cache hits (est. 15-30% additional cost reduction)
2. [Testing Plan](plans/testing-plan.md) — real PR before/after testing (8 PRs across 3 repos) + unit test coverage gaps
3. [Benchmark Tool](plans/benchmark-tool.md) — standalone Go binary for regression testing: golden files, before/after comparison, HTML reports
4. [Auto Verifier Budget](plans/auto-verifier-budget.md) — derive verifier budget from estimated agent cost instead of requiring manual config
5. [Report UI Improvements](plans/report-ui.md) — severity tabs, empty state, improved HTML report
6. [Real PR Validation](plans/real-pr-validation.md) — test verifier against known false-positive PRs to tune prompts

## Completed

1. [Selective Agent Routing](plans/completed/selective-agent-routing.md) — per-file routing, cross-category context, change map, Monkey-style lexer (PR #26)
2. [Git Package](plans/completed/git-package.md) — internal/git/, CloneShallow, ResolveRepoForPR, cross-repo support (PR #26)

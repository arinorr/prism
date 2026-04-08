# Prism Backlog

Pre-release work items. Each links to a detailed plan in `plans/`.

## Priority

1. ~~[Selective Agent Routing](plans/selective-agent-routing.md)~~ — IMPLEMENTED (PR #26: per-file routing + cross-refs + scope hints)
2. [Testing Plan](plans/testing-plan.md) — real PR before/after testing (8 PRs across 3 repos) + 48 new unit/integration tests for coverage gaps
3. [Benchmark Tool](plans/benchmark-tool.md) — standalone Go binary for regression testing: golden files, before/after comparison, HTML reports
4. [Auto Verifier Budget](plans/auto-verifier-budget.md) — derive verifier budget from estimated agent cost instead of requiring manual config
4. [Report UI Improvements](plans/report-ui.md) — improve HTML report readability, navigation, and information density
5. [Real PR Validation](plans/real-pr-validation.md) — test the verifier against known false-positive PRs to tune prompts and resolution

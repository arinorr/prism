# Testing Results: Routing + Verification

## Real PR Before/After Testing

### Test Matrix

| # | Repo | PR | Language | Type | Routing Expected | Status |
|---|------|-----|----------|------|-----------------|--------|
| 1 | arinorr/whetstone | #1 | Go + YAML + Makefile + .md | Mixed (CI, docs, code, tests) | Split across categories, some agents skipped | Pending |
| 2 | arinorr/ari-cloud | #9 | TS + .md + tests | Mixed code + test + config | `__tests__/` routed to Test Engineer, middleware to all | Pending |
| 3 | arinorr/prism | #18 | Go + tests | Code + tests (10 files) | Tests to Test Engineer + Solver, code to all | Pending |
| 4 | arinorr/ari-cloud | #13 | TypeScript | Code (27 files) | All code → all agents, cross-refs between contracts | Pending |
| 5 | arinorr/prism | #22 | Go | Code (15 files) | All code → all agents, verifier catches false positives | Pending |
| 6 | arinorr/prism | #14 | Markdown + .tape | Mixed docs + unknown | README → docs agents, .tape → code → full squad | Pending |
| 7 | arinorr/prism | #23 | Go | Code-only (3 files) | All files → all agents (baseline behavior) | Pending |
| 8 | arinorr/whetstone | #3 | Go + tests | Code + tests (20 files) | Tests routed, verifier cross-refs | Pending |

### Per-PR Results Template

```
### PR #N: [title]

**Baseline** (no routing):
- Agents called: 7/7
- Total input tokens: Xk
- Total cost: $X.XX
- Findings: N (N critical, N warning, N info)

**Routed + Verified**:
- Agents called: N/7 (skipped: ...)
- Files per agent: Know-It-All=N, Architect=N, ...
- Total input tokens: Xk (Δ -X%)
- Total cost: $X.XX (Δ -X%)
- Findings: N (N critical, N warning, N info)
- Dismissed by verifier: N
- Downgraded by verifier: N
- Routing classifications: [per-file list]

**Assessment**: [correct/incorrect routing, false positives caught, regressions]
```

### Success Criteria

| Metric | Target |
|--------|--------|
| Token savings (mixed PRs) | 10-30% reduction |
| Token savings (homogeneous non-code PRs) | 50-70% reduction |
| False positive dismissal rate | > 50% of known false positives |
| False negative rate | 0% — real issues never dismissed |
| Routing accuracy | 100% correct file classification |
| No regressions | Finding quality equal or better than baseline |

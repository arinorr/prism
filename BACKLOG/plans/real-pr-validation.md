# Real PR Validation

## Context

The verifier phase is built and tested with mocks, but hasn't been validated against real-world PRs with known false positives. Before release, we need to confirm it:
1. Actually dismisses false positives we've seen in practice
2. Doesn't dismiss real issues (no false negatives from the verifier)
3. Produces useful verification reasons in the report
4. Stays within reasonable cost bounds

## Goal

Run Prism with `--verify` against a curated set of PRs that have known review outcomes, compare results with and without verification, and tune prompts/thresholds as needed.

## Approach

### Step 1: Curate Test PRs

Collect 5-10 PRs from real repos (Prism itself, or open source projects) that represent common false positive patterns:

| Pattern | Example | Expected verifier behavior |
|---------|---------|---------------------------|
| **Type refinement** | `any` parameter that gets validated upstream | Haiku dismisses — sees the validation in enclosing scope |
| **Nil check elsewhere** | Agent flags potential nil deref, but caller checks before calling | Haiku dismisses — sees nil check in referenced function |
| **Error handling in caller** | Agent says "error not handled" but caller wraps the return | Haiku dismisses — sees error check in calling code |
| **Framework convention** | Agent flags "unused parameter" that's required by an interface | Opus dismisses — needs judgment about framework conventions |
| **Intentional pattern** | Agent flags `//nolint` or deliberately unsafe code with comment | Opus confirms or downgrades — depends on context |
| **Real issue** | Actual SQL injection, actual race condition | Both tiers confirm — verifier should NOT dismiss these |
| **Docs-only PR** | Pure markdown changes | Should produce zero findings (tests routing too) |

### Step 2: Baseline Run

For each test PR, run without verification and capture:
- Raw finding count
- Which findings are clearly false positives (manual review)
- Total cost

```bash
prism review <PR> --format json --stdout > baseline.json
```

### Step 3: Verification Run

Run with verification and compare:

```bash
prism review <PR> --verify --format json --stdout > verified.json
```

Compare:
- How many false positives were dismissed?
- Were any real issues incorrectly dismissed?
- What reasons did the verifier give?
- What was the additional cost?

### Step 4: Prompt Tuning

Based on results, adjust:

- **Haiku system prompt** (`haikuSystemPrompt` in `verify.go`) — if Haiku is too aggressive (dismissing real issues) or too conservative (escalating everything to Opus)
- **Opus system prompt** (`opusSystemPrompt`) — if Opus verdicts aren't well-reasoned
- **Context resolution** — if the resolver isn't providing enough context (e.g., need to follow more reference levels, or increase the 10-reference cap)
- **Scope caps** — `maxScopeLines` (150), `maxReferences` (10), `maxDefLines` (50) — may need adjustment

### Step 5: Metrics

For each test PR, record:

| Metric | Target |
|--------|--------|
| False positive dismissal rate | > 60% of known false positives dismissed |
| False negative rate (real issues dismissed) | 0% — must never dismiss real issues |
| Haiku resolution rate | > 70% of findings resolved by Haiku (not escalated) |
| Verifier cost / agent cost ratio | < 25% |
| Verification time overhead | < 30% of total review time |

### Step 6: Document Results

Create a `BACKLOG/validation-results.md` with:
- Table of test PRs and outcomes
- Before/after comparison
- Any prompt changes made and why
- Remaining false positive patterns not caught

## Deliverables

- Curated set of test PR references (numbers or URLs)
- Baseline vs verified comparison for each
- Prompt adjustments committed if needed
- Confidence assessment: "ready for release" or "needs more work on X"

## When

After items 1-3 (selective routing, auto budget, report UI) are implemented, since those affect the review output the verifier operates on. Validation should be the final gate before release.

## Estimation

- ~2-3 hours of manual testing and comparison
- $5-15 in LLM costs for running against real PRs
- Possible prompt iteration (1-2 rounds)

# Auto Verifier Budget

## Context

The verifier phase has a budget guard (`VerifierBudgetUSD`) that stops LLM calls when the budget is exceeded, letting remaining findings pass through as unverified. Currently it defaults to 0 (no limit) — the user must manually set `--verifier-budget 0.10` or `verifier_budget_usd: 0.10` in config. Without a default, the verifier could theoretically cost more than the agents themselves on a PR with many findings.

We agreed on a default of **20% of estimated agent cost**. This scales naturally with PR size and model selection.

## Goal

When `VerifierBudgetUSD` is 0 (not explicitly set), auto-derive it as 20% of the estimated agent dispatch cost. Surface the computed budget in the estimate output.

## Design

### Extract Cost Estimation

The cost estimation logic currently lives in `cmd/review.go` `printEstimate()` and is tightly coupled to printing. Extract the calculation into a reusable function:

```go
// internal/agents/estimate.go

// EstimateAgentCost returns the projected cost in USD for dispatching
// the given roles against a diff of the given byte size.
func EstimateAgentCost(diffBytes int, roles []Role) float64
```

This uses the same constants (`bytesPerToken=4`, `promptOverheadTokens=2000`, `expectedOutputTokens=1000`) and the same `llm.ModelPricing()` lookup. Move the constants to this file too, so both `printEstimate` and the orchestrator can use them.

### Auto-Budget Derivation

In `Review()`, when building the verifier, compute the auto-budget if none is set:

```go
budget := o.opts.VerifierBudgetUSD
if budget == 0 {
    agentCost := EstimateAgentCost(len(pr.Diff), o.roles)
    budget = agentCost * 0.20
}
```

The verifier already tracks `spent` against `budget` — this just sets the starting value.

### Surface in Estimate

Update `printEstimate()` in `cmd/review.go` to call `EstimateAgentCost()` instead of inline calculation, and show the auto-budget:

```
Est. cost:    ~$0.40
Verifier:     ~$0.08 budget (20% of agent cost, override with --verifier-budget)
```

When the user explicitly sets a budget, show that instead:

```
Verifier:     $0.10 budget (explicit)
```

### Auto-Budget Constant

```go
const DefaultVerifierBudgetRatio = 0.20
```

Define in `internal/agents/estimate.go`. Not configurable via config file for v1 — the ratio is an implementation detail. The user controls the absolute budget via `verifier_budget_usd` if they want to override.

## Files to Create

| File | Purpose |
|------|---------|
| `internal/agents/estimate.go` | `EstimateAgentCost()`, constants, `DefaultVerifierBudgetRatio` |
| `internal/agents/estimate_test.go` | Cost estimation tests |

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/orchestrator.go` `runVerification()` | Compute auto-budget before creating verifier |
| `cmd/review.go` `printEstimate()` | Use `EstimateAgentCost()`, show auto-budget |

## Testing

- Unit: `EstimateAgentCost` with known inputs produces expected cost
- Unit: auto-budget = 20% of estimate when not explicitly set
- Unit: explicit budget overrides auto-budget
- Unit: `printEstimate` output includes verifier budget line

## Estimation

- ~60 lines of new code + tests
- Pure refactoring of existing logic — no behavior change for users who already set explicit budgets

# Token Optimization Strategy

## Current Architecture

Prism runs 7 specialist agents in parallel, each receiving the full PR diff + a skill file as system prompt, followed by a synthesis agent that merges all findings. Total: **8 LLM calls per review**.

For a typical PR with a 50K token diff:
- Each agent: ~50K input + ~5K skill = ~55K input, ~3K output
- 7 agents: ~385K input, ~21K output
- Synthesis: ~20K input (all findings), ~5K output
- **Total: ~405K input + ~26K output per review**

## Token Metrics

As of the token-metrics feature, every review reports:
- Input/output tokens per call
- Cache creation and cache read tokens
- Cost in USD
- Duration

The CLI shows a summary after each review:
```
📊 Tokens: 48k input, 3k output | Cost: $1.23 | Time: 2m30s
```

Full breakdown is available in JSON and HTML report formats.

## Prompt Caching (Already Active)

The Claude CLI automatically uses prompt caching. When the system prompt (skill file) is sent, it's cached for 5 minutes. Key points:

- Cache creation costs a 25% surcharge on those input tokens
- Cache reads get a **90% discount** on cached input tokens
- Each agent has a unique skill file, so skills don't cache across agents
- The diff is in the user prompt — caching behavior for user prompts depends on Claude's internal implementation
- If you run reviews in quick succession (< 5 min apart), subsequent reviews benefit from cached system prompts

This is automatic — no code changes needed.

## Per-Agent Model Selection (Implemented)

Each agent now has a default model tier appropriate to its task complexity:

| Agent | Model | Reasoning |
|-------|-------|-----------|
| Architect | opus | Deep architectural reasoning, cross-file analysis |
| Sentinel | opus | Nuanced security analysis, subtle vulnerability detection |
| Know-It-All | sonnet | Good pattern matching for idioms and best practices |
| Solver | sonnet | Correctness analysis needs decent reasoning |
| Optimizer | sonnet | Performance pattern recognition |
| Editor | haiku | Style, naming, readability are well-scoped tasks |
| Test Engineer | haiku | Test coverage checks are pattern matching |

**Cost impact**: Editor and Test Engineer at haiku pricing (~$0.80/M) vs opus pricing (~$15/M) = ~19x cheaper for those agents.

The `--model` CLI flag overrides all per-role defaults (e.g., `--model opus` forces all agents to use opus).

## Budget Cap (Implemented)

`--max-budget-usd <amount>` caps the maximum dollar spend per agent call. This is passed through to the Claude CLI's `--max-budget-usd` flag.

Example: `prism review 42 --max-budget-usd 0.50` limits each of the 8 calls to $0.50 max.

Can also be set in `.prism.yml`:
```yaml
max_budget_usd: 0.50
```

## Future Optimizations (Not Yet Implemented)

### Diff Compression (Safe)

Strip content that never contributes to code review quality:
- Binary file diffs
- Lock file changes (package-lock.json, yarn.lock, go.sum)
- Generated code (protobuf .pb.go, codegen output)
- Vendor directory changes

**Estimated savings**: 10-30% depending on PR content.
**Risk**: None — this content is never useful for review.

### Context Line Stripping (Moderate Risk)

Unified diffs include 3 lines of unchanged context around each hunk. For some agents, only the changed lines (+/-) matter:
- Safe to strip for: Editor, Test Engineer (focus on what changed)
- Risky to strip for: Architect, Sentinel (need surrounding code for context)

**Estimated savings**: 20-40% of diff size.
**Risk**: May miss issues that depend on surrounding code.

### Two-Pass Triage (High Impact, Higher Effort)

Run a cheap triage pass first (Haiku) that categorizes files by concern area, then dispatch only relevant files to specialist agents.

**Estimated savings**: 50-70% for specialist agents.
**Risk**: Triage model may miscategorize files, causing missed issues.
**Status**: Needs further design work.

### API Migration for Explicit Prompt Caching (Highest Impact)

Switch from `claude --print` CLI to the Anthropic Messages API directly. This enables explicit `cache_control` markers on the shared diff, guaranteeing the 2nd-7th agents pay only 10% for the cached diff tokens.

**Estimated savings**: 60-80% on shared diff tokens across agents.
**Risk**: Significant architectural change (new LLM adapter). The `llm.LLM` interface already supports this pattern.
**Status**: Deferred until CLI-based optimizations are exhausted.

## Pricing Reference

Claude model pricing (verify at docs.anthropic.com for current rates):

| Model | Input (per 1M tokens) | Output (per 1M tokens) |
|-------|----------------------|------------------------|
| Haiku 4.5 | ~$0.80 | ~$4.00 |
| Sonnet 4.6 | ~$3.00 | ~$15.00 |
| Opus 4.6 | ~$15.00 | ~$75.00 |

Prompt caching:
- Cache write: 25% surcharge on input token price
- Cache read: 90% discount on input token price
- Cache TTL: 5 minutes (extended on each hit)

## Quick Wins (No Code Changes)

- Use fewer agents for small PRs: `prism review 123 --roles sentinel,architect,know-it-all`
- Use a cheaper model for everything: `prism review 123 --model sonnet`
- Cap spending: `prism review 123 --max-budget-usd 2.00`

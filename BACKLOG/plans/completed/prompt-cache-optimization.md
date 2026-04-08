# Prompt Cache Optimization

## Context

Benchmark results on whetstone PR #1 showed a 47% cost reduction from routing, but also revealed an important secondary effect: **cache read tokens increased from 272k to 491k** (+81%). Routing made prompts more structurally similar, which improved LLM prompt cache hit rates.

This observation suggests a new class of optimizations: restructure prompts to **maximize shared prefixes** across agents. Every byte that's identical between two agents' prompts is cached and not re-processed. The LLM's prompt cache works on prefix matching — the longer the shared prefix, the higher the cache hit.

**Current cost breakdown for 7 agents:**
- Cache creation: pay full price for new content
- Cache read: pay ~10% of the input token price
- The more content that hits cache, the cheaper each subsequent agent

## Goal

Restructure prompts so agents share the longest possible common prefix. Estimated impact: additional 15-30% cost reduction on top of routing savings, with zero quality impact.

## How LLM Prompt Caching Works

Claude's prompt cache matches on **byte-identical prefixes**. If Agent A's prompt is:

```
[System: skill A] [Instructions] [PR metadata] [Diff: files 1-12]
```

And Agent B's prompt is:

```
[System: skill B] [Instructions] [PR metadata] [Diff: files 1-8]
```

The cache prefix breaks at the first different byte — the system prompt. Everything after is re-processed for Agent B.

But if we restructure:

```
[System: shared instructions + skill A] [PR metadata] [Diff: sorted files]
```

Then Agent B shares the `[PR metadata]` prefix because it's identical content at the same position. The diff also shares a prefix if files are in the same order and B's files are a subset of A's.

## Optimizations

### 1. Move Static Instructions to System Prompt

**Current structure:**
```
System prompt:  [skill file: ~60 lines, agent-specific]
User prompt:    [security warning] [PR title] [PR description] [diff]
                [cross-refs] [scope-hints]
                [JSON format instructions: ~25 lines, IDENTICAL across agents]
```

The JSON format instructions, risk guide, scope guide, and quality instructions are **identical across all 7 agents** (~500 tokens) but sit at the END of the user prompt. They're after the diff, which varies per-agent. This means they're NEVER cached — each agent re-processes 500 identical tokens.

**Proposed structure:**
```
System prompt:  [shared instructions: format + risk + scope + quality]
                [skill file: agent-specific personality]
User prompt:    [PR title] [PR description]
                [scope-hints] [cross-refs]
                [diff: sorted files, variable]
```

Move the 500 tokens of shared instructions into the system prompt, BEFORE the skill file. The system prompt is `--append-system-prompt` in the Claude CLI, which means it's appended to Claude's built-in system prompt. Since all agents share the same instructions prefix, this content is cached after the first agent.

**Savings:** ~500 tokens × 6 agents = ~3,000 tokens saved from re-processing per review.

### 2. Sort Files Alphabetically in AssembleDiff

**Current:** `BuildReviewContext` iterates a `map[string]string` (random order), so `AssembleDiff` produces different file orderings across runs and across agents.

**Fix:** Sort `ClassifiedFile` slices by path before assembling.

```go
func BuildReviewContext(...) *ReviewContext {
    // ... build files ...
    
    // Sort for deterministic output and cache-friendly prefix sharing.
    sort.Slice(files, func(i, j int) bool {
        return files[i].Path < files[j].Path
    })
    
    // ...
}
```

**Why this helps:** If Know-It-All sees `[a.go, b.go, c.go, README.md]` and Solver sees `[a.go, b.go, c.go]`, the shared prefix is `a.go + b.go + c.go` — identical bytes in the same position. If files were in random order, the prefix might break at the first file.

For this to work well, the agent-specific subsets must preserve the global sort order. `FilterFilesForRole` already does this — it iterates the input slice in order and keeps matching files, so the output maintains the input's sort order.

**Savings:** Hard to quantify precisely, but on a 10-file PR where 7 files are shared across agents, this could mean ~70% of the diff content is cached after the first agent.

### 3. Put Variable Content Last

**Current user prompt order:**
```
1. Security warning (identical, ~30 tokens)
2. PR title (identical, ~10 tokens)
3. PR description (identical, ~50 tokens)
4. Diff (VARIABLE per agent — different files)
5. Cross-refs (variable per agent)
6. Scope hints (identical)
7. Instructions (identical, ~500 tokens)
```

The diff (item 4) is the first variable content. Everything before it is a shared prefix. Everything after it is orphaned — even though scope hints and instructions are identical, they're at different byte offsets because the diff length varies per agent.

**Proposed user prompt order:**
```
1. PR title (identical)
2. PR description (identical)
3. Scope hints (identical — derived from ChangeMap, not per-agent)
4. Cross-refs (variable, but small — 0-5 refs, ~150 tokens max)
5. Diff (VARIABLE — largest content, different per agent)
```

The security warning moves to the system prompt (it's static). Instructions move to the system prompt (optimization #1). Scope hints move BEFORE the diff (they're identical across agents). This maximizes the shared prefix:

```
[PR title + description + scope hints] = shared prefix (~100 tokens)
[cross-refs]                           = small variable section
[diff]                                 = large variable section (last)
```

**Savings:** Every agent shares the PR metadata + scope hints prefix. Small but compounds with sorted files.

### 4. Normalize Whitespace

**Current:** The prompt is built with `fmt.Fprintf` and `strings.Builder`, with inconsistent trailing newlines, spaces, and blank lines between sections. Two prompts that are logically identical might differ by a trailing `\n`, breaking the cache prefix.

**Fix:** Add a `normalizePrompt(s string) string` function that:
- Trims trailing whitespace from each line
- Normalizes `\r\n` to `\n`
- Collapses 3+ consecutive newlines to 2
- Ensures the prompt ends with exactly one `\n`

Apply it to both the system prompt and user prompt before sending.

```go
func normalizePrompt(s string) string {
    // ... normalize whitespace ...
}

// In runAgent:
response, usage, err := o.llm.Complete(ctx, llm.Request{
    SystemPrompt: normalizePrompt(skill),
    UserPrompt:   normalizePrompt(prompt),
    // ...
})
```

**Savings:** Small per-prompt, but eliminates random cache misses from whitespace differences.

### 5. Shared System Prompt Prefix

**Current:** Each agent has a completely different system prompt (skill file). The system prompt cache breaks at byte 0 for every agent.

**Proposed:** Prepend a shared prefix to all skill files:

```
[SHARED: instructions + format + risk guide + scope guide + quality guide]
[AGENT-SPECIFIC: skill file content]
```

This means the first ~500 tokens of the system prompt are cached after the first agent. The remaining ~60 lines of skill file are agent-specific, but the shared prefix is "free" for agents 2-7.

Implementation: in `NewOrchestrator`, when loading skills, prepend the shared instructions block:

```go
const sharedInstructions = `... format instructions, risk guide, scope guide ...`

for _, r := range roles {
    data, err := readSkillFile(r.SkillFile, exeDir)
    combined := sharedInstructions + "\n\n" + string(data)
    // ... append language modules ...
    skills[r.Slug] = combined
}
```

**This is the biggest single optimization.** The shared instructions are ~500 tokens, and they're sent 7 times today. With the shared prefix, they're sent once and cached 6 times.

**Savings:** ~500 tokens × 6 agents × cache discount ≈ significant per-review savings.

## Proposed Prompt Structure (After All Optimizations)

### System Prompt (per-agent, but with shared prefix)
```
[SHARED PREFIX — cached after first agent:]
You are reviewing a pull request. Respond with a JSON object containing
an array of findings...
[format instructions]
[risk guide]
[scope guide]
[quality instructions]

[AGENT-SPECIFIC — varies per agent:]
[skill file: personality, lens, what to look for]
[language modules: Go, TypeScript specifics]
```

### User Prompt (per-agent)
```
[SHARED PREFIX — identical across agents:]
<pr-title>Title</pr-title>
<pr-description>Description</pr-description>
<scope-context>
Symbols modified: HandleRequest (handler.go)
Symbols new: ValidateInput (validator.go)
</scope-context>

[SMALL VARIABLE — per-agent cross-refs:]
<cross-references>
Referenced from test files:
  handler.go:HandleRequest() ...
</cross-references>

[LARGE VARIABLE — per-agent diff, sorted alphabetically:]
<pr-diff>
diff --git a/handler.go b/handler.go
...
</pr-diff>
```

## Files to Modify

| File | Change |
|------|--------|
| `internal/agents/orchestrator.go` | Extract shared instructions from `buildAgentPrompt` into a constant. Prepend to skill files in `NewOrchestrator`. Restructure `buildAgentPrompt` to put metadata first, diff last. Move security warning to system prompt. Add `normalizePrompt`. |
| `internal/agents/routing.go` | Sort `ClassifiedFile` slices by path in `BuildReviewContext`. |
| `internal/agents/orchestrator_test.go` | Update prompt structure tests. |
| `internal/agents/prompt_test.go` | Update for new section ordering. |

## Testing

- Unit: `normalizePrompt` strips trailing whitespace, collapses newlines
- Unit: `BuildReviewContext` produces files sorted by path
- Unit: `buildAgentPrompt` puts metadata before diff
- Unit: system prompt starts with shared instructions prefix
- Unit: two agents' system prompts share the instructions prefix
- Integration: verify prompt byte comparison — two agents' prompts share expected prefix length
- Benchmark: re-run whetstone PR #1, compare cache_creation vs cache_read tokens before/after

## Measuring Impact

Run the benchmark before and after:
```bash
./prism-before review <PR> --format json --stdout --yes > before.json 2> before.log
./prism-after review <PR> --format json --stdout --yes > after.json 2> after.log
```

Compare:
- `cache_creation_input_tokens` — should decrease (less new content)
- `cache_read_input_tokens` — should increase (more cache hits)
- `cost_usd` — should decrease
- Finding count + score — should be unchanged (no quality impact)

## Implementation Order

1. **Sort files in BuildReviewContext** — 3 lines, immediate cache benefit
2. **Move instructions to system prompt prefix** — biggest impact, ~30 lines
3. **Reorder user prompt: metadata first, diff last** — ~20 lines refactor
4. **Add normalizePrompt** — ~15 lines, eliminates edge case cache misses
5. **Move security warning to system prompt** — 5 lines

Steps 1-2 are the highest ROI. Steps 3-5 are incremental gains.

## Estimation

- ~100 lines of changes (small, focused refactors)
- No new files, no new dependencies
- Zero quality impact — same content, different ordering
- Expected savings: 15-30% cost reduction on top of routing savings

## Risk

The main risk is that reordering prompt sections changes LLM behavior. The diff being at the end vs the middle *could* affect how the model weights it. Mitigation: run the benchmark before and after, compare finding quality. If findings change significantly, the ordering may need adjustment.

The other risk is that Claude's cache implementation details could change. These optimizations are based on current prefix-matching behavior. If Claude moves to content-addressable caching, the ordering optimizations become irrelevant (but also harmless).

# Prism Backlog

## Up Next

### Review modes (control depth, tokens, and time)
- **Quick** — text-only, diff-only prompts (current behavior). Fastest and cheapest. Good for small PRs and rapid iteration.
- **Standard** — agents can read files from the repo beyond the diff to understand full context (requires Agent SDK migration). Better for medium PRs.
- **Deep** — agents get web search to look up docs, check CVEs, verify API contracts. Most thorough but slowest and most expensive.
- `--mode quick|standard|deep` flag to select
- Token budget per mode: quick ~100K, standard ~300K, deep ~500K+
- Time estimates: quick ~1-2min, standard ~3-5min, deep ~5-10min

### Codebase Expert role
- A new agent that reads the wider codebase (not just the diff) to understand the project structure, patterns, conventions, and architecture
- Provides feedback based on: does this change fit how the rest of the codebase works? Are there existing utilities being reinvented? Does it follow the project's conventions?
- Distinct from the Architect (who thinks about abstract design) — the Codebase Expert has *read the actual code* and knows the specifics
- Requires Standard or Deep mode (needs file read access beyond the diff)
- Could pre-index the repo structure and key files to stay within token budget

### Token usage reporting
- Show estimated token usage before running (`--estimate` flag)
- Show actual token usage after running (input/output per agent + synthesis)
- Show cost estimate based on model pricing
- Surface in the report: "This review used ~130K tokens (~$X)"
- Verbose mode shows per-agent breakdown

### Incremental / follow-up reviews
- Track previous review results per PR (store in `results/` or `.prism/`)
- On re-review, diff the findings: what's fixed, what's new, what persists
- Show a delta report: "3 critical issues fixed, 2 new warnings, 5 unchanged"
- `prism review 42 --follow-up` to explicitly compare with last review
- Skip re-reviewing files that haven't changed since last review
- Useful for the fix-review-fix cycle

### Agentic review (Agent SDK migration)
- Migrate from `claude --print` subprocesses to the Claude Agent SDK
- Enables agents to: read files beyond the diff, search the codebase, browse docs, use web search
- Prerequisite for Standard/Deep review modes and the Codebase Expert role
- Would also enable: streaming progress, multi-turn agent conversations, tool use
- Agents could verify their own findings (e.g. "does this function exist?", "what does this API actually accept?")
- Significant architectural change — the orchestrator would manage agent sessions instead of one-shot prompts

### Language-specific agent skills
- Current skills are language-agnostic — agents give generic advice regardless of whether the code is Go, Python, TypeScript, etc.
- Add language-specific skill variants that encode the idioms, best practices, and common mistakes for each language
- Structure: `skills/go/know-it-all.md`, `skills/python/know-it-all.md`, `skills/typescript/sentinel.md`, etc.
- Auto-detect language from file extensions in the PR diff and load the right skill variant
- Each language variant inherits the base skill and adds language-specific checks:
  - **Go**: `errors.Is()` not `==`, interface size (Pike's proverb), context propagation, table-driven tests, `defer` error handling
  - **Python**: bare `except:`, missing type hints, mutable default args, `__init__` complexity
  - **TypeScript**: `any` abuse, missing null checks, async/await pitfalls, proper typing
  - **Rust**: ownership patterns, lifetime annotations, unsafe blocks, error handling with `?`
- Applies to all agents, not just Know-It-All — Sentinel has different patterns per language (SQL injection in Python vs command injection in Go), Test Engineer knows pytest vs go test conventions, etc.
- Fall back to generic skill if no language-specific variant exists

### Configurable model and settings
- Let users pick which Claude model agents use
- Set max token budget per agent
- Configure agent timeouts
- Support a `.prism.yml` config file per-repo

### Better error recovery
- Retry failed agents once before giving up
- Graceful degradation: if 1-2 agents fail, still produce a review
- Timeout handling for hung agents

### Agent weighting and deduplication
- When multiple agents flag the same issue, merge and show vote count
- Let users weight roles (e.g. prioritize Sentinel for security-sensitive repos)
- Confidence scoring based on agent agreement

## Ideas

### `prism init`
- Generate a `.prism.yml` config for a repo
- Interactive wizard to pick default roles, format, and behavior

### Custom roles
- Let users define their own reviewer personas via markdown skill files
- `--skill-dir` flag to point to a directory of custom skills

### PR size guardrails
- Warn when a PR diff is too large for effective review
- Suggest splitting large PRs
- Chunk large diffs and review in sections

### Review history
- Track reviews over time per-repo
- Show trends (are reviews getting cleaner?)
- `prism history` command

### Slack/Discord integration
- Post review summaries to a channel
- Mention authors when critical issues found

### VS Code extension
- Review current branch changes from the editor
- Inline annotations from findings

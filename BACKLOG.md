# Prism Backlog

## Up Next

### Review modes (control depth, tokens, and time)
- **Quick** — text-only, diff-only prompts (current behavior). Fastest and cheapest. Good for small PRs and rapid iteration.
- **Standard** — agents can read files from the repo beyond the diff to understand full context (requires Agent SDK migration). Better for medium PRs.
- **Deep** — agents get web search to look up docs, check CVEs, verify API contracts. Most thorough but slowest and most expensive.
- `--mode quick|standard|deep` flag to select
- Token budget per mode: quick ~100K, standard ~300K, deep ~500K+
- Time estimates: quick ~1-2min, standard ~3-5min, deep ~5-10min

### Verification Agent (replaces Codebase Expert + Fact Checker)
- A post-review agent that reads the actual codebase to **validate findings** from the 7 specialist agents
- Runs after initial review + dedup, before final scoring
- For each finding, checks whether the claim holds against the actual code:
  - "Finding says X is unsanitized" → check if the rendering layer already handles it
  - "Finding says add validation" → check if validation already exists upstream
  - "Finding says this could XSS" → trace the data flow to the actual rendering context
- **Must output specific evidence** (file path, line number, actual code) so humans can spot-check the verification, not just the conclusion. If the agent can't produce evidence, the finding stands.
- Collapses two previously separate roles (see docs/agent-blind-spots.md for why):
  - Codebase Expert: reads the wider codebase for context
  - Fact Checker: verifies claims against actual code
  - These are the same operation — you can't verify claims without reading the code
- Requires Standard or Deep mode (needs file read access beyond the diff)
- Could pre-index the repo structure and key files to stay within token budget
- Future: support data flow annotations (SECURITY.md or inline comments) for structural verification

### Token usage reporting — remaining
- Future: refine `--estimate` heuristic with real-world calibration data
- Future: per-agent breakdown in JSON report (currently only aggregate)

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
- Prerequisite for Standard/Deep review modes and the Verification Agent
- Would also enable: streaming progress, multi-turn agent conversations, tool use
- Agents could verify their own findings (e.g. "does this function exist?", "what does this API actually accept?")
- Significant architectural change — the orchestrator would manage agent sessions instead of one-shot prompts

### Language-specific agent skills — expand to more agents and languages
- **Done (PR #9)**: Dynamic skill toolbox for Know-It-All and Sentinel with Go and TypeScript/JavaScript modules. Auto-detects language from file extensions and appends matching `skills/{agent}/{language}.md` modules.
- Add modules for more agents: Test Engineer (pytest vs go test vs RTL conventions), Optimizer (language-specific perf patterns), Editor (language-specific readability idioms)
- Add more languages: Python, Rust
  - **Python**: bare `except:`, missing type hints, mutable default args, `__init__` complexity, Django/Flask security patterns
  - **Rust**: ownership patterns, lifetime annotations, unsafe blocks, error handling with `?`

## Done

### Configurable model and settings (PR #11)
- `.prism.yml` config file with layered precedence (defaults < file < CLI)
- `--model`, `--timeout`, `--max-retries`, `--max-budget-usd` flags
- Per-role preferred models (opus for Architect/Sentinel, sonnet for others, haiku for Editor)

### Better error recovery (PR #11)
- Retry failed agents with configurable `--max-retries` (default: 1)
- Graceful degradation: partial results if some agents fail
- Per-agent timeout with `--timeout` flag

### Token usage reporting
- Actual token usage shown after running (total input/output + cost + time)
- Per-agent timing streamed in verbose mode during review
- Per-agent summary table printed after review in verbose mode
- `--estimate` flag for pre-flight token/cost estimate from diff size
- Usage surfaced in markdown and HTML report footers
- JSON report includes full usage breakdown
- Cost calculated from Claude CLI response envelope

### Code quality improvements — Thorsten Ball style (PRs #18, #19, #20)
- **2d.** io.Writer defaults at construction — no nil checks needed
- **2e.** DryRun returns data, caller formats — separation of data and presentation
- **2f.** Attempted sentinel values, reverted to `*int` — pointers are the right tool here
- **2g.** Extract parser.go from orchestrator — each file has one job
- **2h.** Split report package into data + generators — separate concerns
- **2i.** Markdown report via text/template with embed.FS — separate shape from logic
- **Pike.** Inject GitHub client via parameter — no package-level mutable state

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

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

### Language-specific agent skills — expand to more agents and languages
- **Done (PR #9)**: Dynamic skill toolbox for Know-It-All and Sentinel with Go and TypeScript/JavaScript modules. Auto-detects language from file extensions and appends matching `skills/{agent}/{language}.md` modules.
- Add modules for more agents: Test Engineer (pytest vs go test vs RTL conventions), Optimizer (language-specific perf patterns), Editor (language-specific readability idioms)
- Add more languages: Python, Rust
  - **Python**: bare `except:`, missing type hints, mutable default args, `__init__` complexity, Django/Flask security patterns
  - **Rust**: ownership patterns, lifetime annotations, unsafe blocks, error handling with `?`

### Configurable model and settings
- Let users pick which Claude model agents use
- Set max token budget per agent
- Configure agent timeouts
- Support a `.prism.yml` config file per-repo

### Better error recovery
- Retry failed agents once before giving up
- Graceful degradation: if 1-2 agents fail, still produce a review
- Timeout handling for hung agents

### Code quality improvements (Thorsten Ball style)

Idiomatic Go improvements following Thorsten Ball's principles (simple, explicit,
no magic, make the zero value useful). These are structural refactors that don't
change behavior.

**2d. Fill in io.Writer defaults at construction**
- Add `NewOptions()` that sets `Out=os.Stdout`, `ErrOut=os.Stderr`
- Remove nil checks in `out()` and `errOut()` methods on Orchestrator
- Principle: make the zero value useful — writers should never be nil
- Files: `internal/agents/orchestrator.go`, `cmd/review.go`

**2e. DryRun returns data, caller formats**
- `planDryRun()` returns a `DryRunResult` struct with role count, diff bytes,
  sample prompt, model, timeout
- `cmd/review.go` handles formatting/printing the dry run output
- Principle: separate data from formatting — functions return data, callers render
- Files: `internal/agents/orchestrator.go`, `cmd/review.go`

**2f. Config sentinel values instead of pointers**
- Replace `MaxRetries *int` and `DiffContextLines *int` with plain `int`
- Use -1 as "not set" sentinel instead of nil pointer indirection
- Remove `IntPtr()` helper function
- Principle: simpler types, no pointer indirection for optional values
- Files: `internal/config/config.go`, `cmd/review.go`

**2g. Extract parser.go from orchestrator**
- Move parsing logic into its own file `parser.go` (same `agents` package)
- Includes: `rawFinding`, `rawFeedback`, `parseFeedback`, `tryParseStrategies`,
  `parseDirectJSON`, `parseCodeBlock`, `parseJSONMarker`, `NormalizeFinding`,
  `truncateUTF8`
- Orchestrator just calls `parseFeedback()` — same API, better file organization
- Principle: each file has one job
- Files: `internal/agents/orchestrator.go` → `internal/agents/parser.go`

**2h. Split report package into data + generators**
- Separate data preparation from rendering across multiple files:
  - `data.go` — Data struct, file grouping, scope splitting, severity counting
  - `generator.go` — shared types (htmlFinding, htmlFileGroup, etc.)
  - `markdown.go` — `Markdown()` function
  - `html.go` — `HTML()` function + template string + CSS
  - `json.go` — `JSON()` function
- Each generator takes prepared data and renders one format
- Adding new formats (SARIF, TUI) means adding one file
- Principle: separate concerns, each file has one responsibility
- Files: `internal/report/report.go` → split into 5 files

**2i. Markdown report via text/template (depends on 2h)**
- Replace ~100 lines of `fmt.Fprintf` calls with a `text/template`
- Same pattern as the HTML report: data in, rendered output out
- Template string is the "shape" of the markdown, readable at a glance
- Principle: separate what to render from how to render it
- Files: `internal/report/markdown.go`

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

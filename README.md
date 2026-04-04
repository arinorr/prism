# Prism

A multi-agent PR review tool that passes your code through 7 specialized lenses. Each agent reviews your changes from a distinct perspective, then findings are deduplicated, scored, and combined into a single actionable report.

## How it works

```
PR Diff ──> compress & strip noise
                │
                v
            7 Claude agents review in parallel
                │
                ├── Know-It-All   (best practices, idioms, code smells)
                ├── Architect     (system design, patterns, scalability)
                ├── Solver        (correctness, edge cases, completeness)
                ├── Editor        (readability, simplicity, clarity)
                ├── Optimizer     (performance, complexity, allocations)
                ├── Sentinel      (security, vulnerabilities, OWASP)
                └── Test Engineer (test coverage, edge cases, flaky tests)
                │
                v
            Deduplicate findings, merge votes
                │
                v
            Health score (0–100), grade (A+ → F), verdict
                │
                v
            Report (plain, markdown, HTML, or JSON)
```

Single-pass AI review has blind spots. Prism uses multiple specialized perspectives to catch more issues and produce higher-quality feedback.

## Quick Start

```bash
# Build
go build -o prism .

# Review a PR (requires gh CLI and Claude Code)
prism review 42

# Review with a specific model
prism review 42 --model opus

# Review with specific roles only
prism review 42 --roles sentinel,solver,test-engineer

# Dry run (see what would happen without calling Claude)
prism review 42 --dry-run
```

## Install

Requires **Go 1.24+**, the **gh CLI** (authenticated), and **Claude Code**.

```bash
git clone https://github.com/arinorr/prism.git
cd prism
go build -o prism .

# Optional: move to your PATH
sudo mv prism /usr/local/bin/
```

## Commands

### `prism review <pr-number|pr-url>`

Review a pull request. Dispatches all agents in parallel, deduplicates their findings, computes a health score, and outputs the result.

```bash
prism review 42                           # HTML report -> results/ (default)
prism review 42 --format plain            # Plain text to terminal
prism review 42 --format md               # Markdown report -> results/
prism review 42 --format json             # JSON report -> results/
prism review 42 --format md --stdout      # Print to terminal instead of file
prism review 42 --comment                 # Post inline comments on the PR
prism review 42 --roles sentinel,solver   # Only run specific agents
prism review 42 --model haiku             # Use a specific Claude model
prism review 42 --verbose                 # Show timing and debug info
prism review 42 --dry-run                 # Preview without calling Claude
```

### `prism version`

Print the version.

### `prism help`

Show help and available options.

## Options

| Flag | Description |
|------|-------------|
| `--format` | Output format: `plain`, `md`, `html`, `json` (default: `html`) |
| `--comment` | Post warning/critical findings as inline PR comments |
| `--roles` | Comma-separated list of roles (default: all) |
| `--model` | Claude model: `sonnet`, `opus`, `haiku` |
| `--timeout` | Per-agent timeout as a Go duration (default: `5m`) |
| `--max-retries` | Number of retries per agent on failure (default: `1`) |
| `--max-budget-usd` | Maximum dollar spend per agent call (e.g. `0.50`) |
| `--config` | Path to config file (default: `.prism.yml`) |
| `--no-compress` | Disable diff compression (send raw diff to agents) |
| `--stdout` | Print report to terminal instead of saving to `results/` |
| `-y`, `--yes` | Skip confirmation prompts (e.g. large diff warning) |
| `-v`, `--verbose` | Show prompts, timing, token usage, and response details |
| `--dry-run` | Preview what agents would run without calling Claude |

## Configuration

Prism looks for a `.prism.yml` file in the current directory (override with `--config`). All fields are optional — CLI flags take precedence.

```yaml
# .prism.yml
roles: [sentinel, solver, test-engineer]
model: sonnet
format: html
agent_timeout: 5m
max_retries: 1
max_budget_usd: 0.50

# Diff compression
no_compress: false
diff_context_lines: 1    # Lines of context around changes (1-3, or -1 for all)
strip_patterns:           # Additional glob patterns to strip from diffs
  - "*.generated.go"
  - "docs/**"

# Size thresholds (0 to disable)
diff_warn_bytes: 153600   # Warn at 150 KB (default)
diff_chunk_bytes: 307200  # Suggest splitting at 300 KB (default)
```

## Reviewer Roles

| Role | Focus |
|------|-------|
| **Know-It-All** | Best practices, code smells, language idioms, naming |
| **Architect** | System fit, abstractions, coupling, dependency direction |
| **Solver** | Does the PR solve its stated problem? Edge cases, error paths |
| **Editor** | Readability, simplicity, cognitive load, duplication |
| **Optimizer** | Big-O complexity, allocations, caching, I/O efficiency |
| **Sentinel** | Injection, auth gaps, secrets, SSRF, OWASP top 10 |
| **Test Engineer** | Test coverage, missing edge cases, flaky tests, regression |

Agents automatically detect languages in the diff (currently Go and TypeScript/JavaScript) and load language-specific skill modules for deeper, idiomatic feedback.

## Health Score and Grading

After deduplication, Prism computes a **health score** (0–100) based on the severity, scope, and consensus of findings:

| Grade | Score | Verdict |
|-------|-------|---------|
| A+ | 95–100 | Approve |
| A | 90–94 | Approve |
| B+ | 80–89 | Approve with suggestions |
| B | 70–79 | Approve with suggestions |
| C | 60–69 | Request changes |
| D | 40–59 | Request changes |
| F | 0–39 | Needs discussion |

Findings scoped to **changed lines** are weighted more heavily than those about existing or codebase-level code. When multiple agents flag the same issue, their votes increase the finding's impact.

## Deduplication

When multiple agents report the same issue, Prism merges them into a single finding with a **vote count** showing how many agents agreed. Two findings are considered duplicates when they target the same file, are within 5 lines of each other, and have similar summaries (Jaccard similarity ≥ 0.4). Deduplicated findings are sorted by vote count, then severity.

## Diff Compression

By default, Prism compresses diffs before sending them to agents to reduce token usage and cost:

- **Strips** lock files, generated/compiled code, binaries, and vendor/node_modules
- **Reduces context** to 1 line around changes (configurable via `diff_context_lines`)
- **Custom patterns** can be added via `strip_patterns` in `.prism.yml`

Use `--no-compress` to send the raw diff instead. After a review, token usage and cost are displayed:

```
Tokens: 142k input, 18k output | Cost: $0.34 | Time: 1m12s
```

Use `--verbose` for per-agent breakdowns.

## Reports

Reports are saved to `results/` by default:

```
results/prism-pr-42.html   (default)
results/prism-pr-42.md
results/prism-pr-42.json
```

HTML reports include:
- Health score gauge with letter grade and verdict
- Dashboard summary cards
- Severity stats (critical / warning / info counts)
- Deduplicated findings grouped by file with vote counts and severity badges

Use `--stdout` to pipe reports to other tools instead.

## Large Diffs

When a diff exceeds **150 KB**, Prism warns that review quality may be reduced. At **300 KB**, it suggests splitting the PR into smaller pieces. Use `-y` / `--yes` to skip these prompts in CI. Thresholds are configurable in `.prism.yml`.

## GitHub Action

Prism ships as a GitHub Action for automated PR reviews:

```yaml
- uses: arinorr/prism@main
  with:
    anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
    format: md
    comment: true
```

See [docs/github-action.md](docs/github-action.md) for full configuration.

## Development

```bash
make build       # Build the binary
make test        # Run tests with race detector
make lint        # Run golangci-lint
make check       # All quality checks (lint + test + build)
make clean       # Remove build artifacts
make review PR=2 # Build and review a PR
```

## License

MIT

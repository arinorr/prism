# Prism

A multi-agent PR review tool that passes your code through 7 specialized lenses. Each agent reviews your changes from a distinct perspective, then a synthesizer combines their feedback into a single actionable report.

## How it works

```
PR Diff ──> 7 Claude agents review in parallel
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
            Synthesizer combines findings, resolves conflicts
                │
                v
            Report (terminal, markdown, HTML, or JSON)
```

Single-pass AI review has blind spots. Prism uses multiple specialized perspectives to catch more issues and produce higher-quality feedback.

## Quick Start

```bash
# Build
go build -o prism .

# Review a PR (requires gh CLI and Claude Code)
prism review 42

# Generate an HTML report
prism review 42 --format html

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

Review a pull request. Dispatches all agents in parallel, synthesizes their feedback, and outputs the result.

```bash
prism review 42                           # Terminal summary
prism review 42 --format md               # Markdown report -> results/
prism review 42 --format html             # HTML report -> results/
prism review 42 --format json             # JSON report -> results/
prism review 42 --format md --stdout      # Print to terminal instead of file
prism review 42 --comment                 # Post inline comments on the PR
prism review 42 --roles sentinel,solver   # Only run specific agents
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
| `--comment` | Post warning/critical findings as inline PR comments |
| `--roles` | Comma-separated list of roles (default: all) |
| `--format` | Output format: `md`, `html`, `json` (default: terminal summary) |
| `--stdout` | Print report to terminal instead of saving to `results/` |
| `-v`, `--verbose` | Show prompts, timing, and response details |
| `--dry-run` | Preview what agents would run without calling Claude |

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

## Reports

Reports are saved to `results/` by default:

```
results/prism-pr-42.md
results/prism-pr-42.html
results/prism-pr-42.json
```

HTML reports include:
- Severity stats bar (critical / warning / info counts)
- Findings grouped by file with colored severity badges
- Synthesized summary from all agents
- Inline suggestion count

Use `--stdout` to pipe reports to other tools instead.

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

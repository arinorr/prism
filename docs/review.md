# prism review

Review a pull request with multiple specialized agents.

## Synopsis

```
prism review <pr-number|pr-url> [flags]
```

## Description

Dispatches 7 reviewer agents in parallel, each analyzing the PR diff through a specialized lens. After all agents complete, a synthesizer combines their findings into a single report.

The PR can be specified as a number (e.g. `42`) or a GitHub URL.

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--format <fmt>` | | Output format: `md`, `html`, `json`. Reports save to `results/` |
| `--stdout` | | Print report to terminal instead of saving to file |
| `--comment` | | Post warning/critical findings as inline PR comments |
| `--roles <list>` | | Comma-separated roles to use (default: all) |
| `--verbose` | `-v` | Show prompts, timing, raw responses |
| `--dry-run` | | Preview without calling Claude |

## Examples

### Basic review

```bash
prism review 42
```

Prints a synthesized summary to the terminal.

### Generate reports

```bash
# Markdown report
prism review 42 --format md
# -> results/prism-pr-42.md

# HTML report with styled UI
prism review 42 --format html
# -> results/prism-pr-42.html

# Structured JSON
prism review 42 --format json
# -> results/prism-pr-42.json
```

### Post inline comments

```bash
prism review 42 --comment
```

Posts warning and critical findings as inline comments on the PR. Requires `gh` CLI with write access. Info-level findings are excluded to avoid noise.

### Use specific roles

```bash
# Security-focused review
prism review 42 --roles sentinel

# Quick review with just 3 agents
prism review 42 --roles solver,sentinel,test-engineer
```

Available roles: `know-it-all`, `architect`, `solver`, `editor`, `optimizer`, `sentinel`, `test-engineer`.

### Dry run

```bash
prism review 42 --dry-run --verbose
```

Shows which agents would run, the prompt that would be sent, and skill file sizes — without making any API calls.

### Pipe to other tools

```bash
# Pipe markdown to a pager
prism review 42 --format md --stdout | less

# Pipe JSON to jq
prism review 42 --format json --stdout | jq '.findings[] | select(.severity == "critical")'
```

## Output

By default, `prism review` prints a synthesized summary to the terminal. When `--format` is specified, reports are saved to the `results/` directory with the naming convention `prism-pr-{number}.{ext}`.

Use `--stdout` to override file output and print to the terminal instead.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Review completed successfully |
| 1 | Error (failed to fetch PR, all agents failed, etc.) |

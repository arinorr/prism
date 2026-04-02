# Reports

Prism generates review reports in three formats: Markdown, HTML, and JSON.

## Output behavior

By default, `prism review` prints a synthesized summary to the terminal. When `--format` is specified, reports are saved to the `results/` directory:

```bash
prism review 42 --format md    # -> results/prism-pr-42.md
prism review 42 --format html  # -> results/prism-pr-42.html
prism review 42 --format json  # -> results/prism-pr-42.json
```

Use `--stdout` to print to the terminal instead of writing a file:

```bash
prism review 42 --format json --stdout | jq '.findings'
```

The `results/` directory is created automatically and is listed in `.gitignore`.

## Markdown

The markdown report includes:

- PR metadata (number, title, files changed, agents used, duration)
- Synthesized summary from all agents
- Findings grouped by file with severity, agent, and line number
- Detailed explanations for warning and critical findings
- Inline suggestion count

Markdown reports render well on GitHub (step summaries, PR comments, wikis).

## HTML

The HTML report is a standalone page with:

- **Stats bar** — at-a-glance counts for critical, warning, info, and suggestions
- **Synthesis summary** — the combined agent feedback rendered from markdown
- **File groups** — collapsible sections per file with severity badges
- **Finding cards** — each finding shows severity, line number, agent, summary, and detail

All user-derived content is HTML-escaped to prevent XSS.

## JSON

The JSON report is structured for machine consumption:

```json
{
  "pr": {
    "number": "42",
    "title": "Fix the widget",
    "files": 3
  },
  "summary": "Overall assessment...",
  "findings": [
    {
      "file": "widget.go",
      "line": 10,
      "severity": "critical",
      "summary": "Nil pointer dereference",
      "detail": "Check for nil before...",
      "role": "solver"
    }
  ],
  "suggestions": [
    {
      "file": "widget.go",
      "line": 10,
      "body": "**[critical]** Nil pointer...",
      "role": "solver"
    }
  ],
  "roles": ["Solver", "Sentinel"],
  "duration": "45s"
}
```

Empty arrays serialize as `[]`, not `null`.

### Filtering with jq

```bash
# Critical findings only
prism review 42 --format json --stdout | jq '.findings[] | select(.severity == "critical")'

# Count by severity
prism review 42 --format json --stdout | jq '.findings | group_by(.severity) | map({severity: .[0].severity, count: length})'

# Files with most findings
prism review 42 --format json --stdout | jq '.findings | group_by(.file) | map({file: .[0].file, count: length}) | sort_by(-.count)'
```

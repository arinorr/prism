# GitHub Action

Prism ships as a GitHub Action for automated PR reviews in CI.

## Usage

```yaml
name: Prism Review
on:
  pull_request:
    branches: [main]

permissions:
  contents: read
  pull-requests: write

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: arinorr/prism@main
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `anthropic_api_key` | Yes | | Anthropic API key for Claude |
| `roles` | No | `""` (all) | Comma-separated list of reviewer roles |
| `format` | No | `md` | Output format: `md`, `html`, `json` |
| `comment` | No | `false` | Post inline suggestions as PR comments |

## Outputs

| Output | Description |
|--------|-------------|
| `report` | Path to the generated report file |
| `findings_count` | Total number of findings (JSON format only) |

## Examples

### Basic review with markdown summary

```yaml
- uses: arinorr/prism@main
  with:
    anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
    format: md
```

The markdown report is automatically posted to the GitHub Actions step summary.

### Post inline comments

```yaml
- uses: arinorr/prism@main
  with:
    anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
    comment: true
```

Warning and critical findings are posted as inline comments on the PR.

### Security-only review

```yaml
- uses: arinorr/prism@main
  with:
    anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
    roles: sentinel
    comment: true
```

### JSON output for downstream processing

```yaml
- uses: arinorr/prism@main
  id: review
  with:
    anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
    format: json

- name: Check for critical findings
  run: |
    CRITICAL=$(jq '[.findings[] | select(.severity == "critical")] | length' ${{ steps.review.outputs.report }})
    if [ "$CRITICAL" -gt 0 ]; then
      echo "::error::Found $CRITICAL critical findings"
      exit 1
    fi
```

## Artifacts

The action automatically uploads the review report as a GitHub Actions artifact named `prism-review`. This is available for download from the Actions tab.

## Permissions

The action needs:
- `contents: read` to check out the repository
- `pull-requests: write` if using `comment: true` to post inline comments

## Requirements

The action installs its own dependencies:
- Go 1.24 (via `actions/setup-go`)
- Claude Code (via npm)

You only need to provide the `ANTHROPIC_API_KEY` secret.

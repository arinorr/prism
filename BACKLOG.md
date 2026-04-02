# Shinobi Backlog

## In Progress

_(nothing — all current work tracked in PRs)_

## Up Next

### Configurable model and settings
- Let users pick which Claude model agents use
- Set max token budget per agent
- Configure agent timeouts
- Support a `.shinobi.yml` config file per-repo

### Better error recovery
- Retry failed agents once before giving up
- Graceful degradation: if 1-2 agents fail, still produce a review
- Timeout handling for hung agents

### Agent weighting and deduplication
- When multiple agents flag the same issue, merge and show vote count
- Let users weight roles (e.g. prioritize Sentinel for security-sensitive repos)
- Confidence scoring based on agent agreement

## Ideas

### `shinobi init`
- Generate a `.shinobi.yml` config for a repo
- Interactive wizard to pick default roles, format, and behavior

### Custom roles
- Let users define their own reviewer personas via markdown skill files
- `--skill-dir` flag to point to a directory of custom skills

### PR size guardrails
- Warn when a PR diff is too large for effective review
- Suggest splitting large PRs
- Chunk large diffs and review in sections

### Caching and incremental review
- Cache agent results so re-running on the same diff is instant
- Incremental review: only re-review changed files on force-push

### Review history
- Track reviews over time per-repo
- Show trends (are reviews getting cleaner?)
- `shinobi history` command

### Slack/Discord integration
- Post review summaries to a channel
- Mention authors when critical issues found

### VS Code extension
- Review current branch changes from the editor
- Inline annotations from findings

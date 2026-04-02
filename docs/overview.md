# Prism Overview

Prism is a multi-agent code review tool that analyzes pull requests through 7 specialized perspectives. Instead of one general-purpose reviewer, Prism dispatches a team of focused agents — each trained on a specific aspect of code quality — then synthesizes their feedback into a single coherent report.

## Why multi-agent review?

A single AI reviewer tends to produce shallow, generic feedback. It tries to cover everything and ends up going deep on nothing. Prism solves this by giving each agent a narrow focus:

- The **Sentinel** only looks for security vulnerabilities
- The **Optimizer** only thinks about performance
- The **Test Engineer** only evaluates test coverage

This specialization means each agent produces more targeted, actionable findings than a generalist would. When multiple agents flag the same issue independently, that's a strong signal it matters.

## Architecture

```
                    ┌─────────────────┐
                    │   prism review   │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Fetch PR diff   │
                    │  (gh CLI / git)  │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
         ┌────▼────┐  ┌─────▼─────┐  ┌────▼────┐
         │ Agent 1  │  │ Agent 2   │  │ Agent N │  (parallel)
         │ + skill  │  │ + skill   │  │ + skill │
         └────┬────┘  └─────┬─────┘  └────┬────┘
              │              │              │
              └──────────────┼──────────────┘
                             │
                    ┌────────▼────────┐
                    │   Synthesizer    │
                    │ (resolves conflicts, │
                    │  ranks findings)     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Report output   │
                    │ (terminal/md/html/json) │
                    └─────────────────┘
```

### Agent dispatch

Each agent runs as a separate `claude --print` invocation with:
- The PR diff as the main prompt
- A skill file (markdown persona) appended as a system prompt
- Instructions to output structured JSON findings

All agents run in parallel. As each completes, Prism reports progress.

### Synthesis

The synthesizer receives all agent findings and produces:
- An overall assessment
- Findings grouped by file and priority
- Conflict resolution when agents disagree
- A final verdict (approve, request changes, needs discussion)

### Skill files

Each agent's behavior is defined by a markdown skill file in `skills/`. These can be customized or extended. See [roles.md](roles.md) for details on each role.

## Requirements

- **Go 1.24+** — to build Prism
- **gh CLI** — to fetch PR metadata and diffs (with git fallback)
- **Claude Code** — the Claude CLI, which Prism invokes for each agent

## Token usage

A typical review with 7 agents on a medium PR (~5-10KB diff) uses approximately:
- **Input:** ~70-85K tokens (7 agents x ~10K prompt + synthesis)
- **Output:** ~50-60K tokens (agent responses + synthesis)
- **Total:** ~120-150K tokens per review

Larger PRs or more agents increase this proportionally.

# Benchmark Tool

## Context

The current benchmark is a bash script (`scripts/benchmark.sh`) with several issues: jq query bugs, silent failures, hardcoded PR list, no regression detection, fragile heredoc HTML generation. We need a proper tool for validating pipeline changes.

## Goal

A standalone Go binary (`cmd/benchmark/`) that runs Prism against a configurable set of PRs, compares results against golden files, and generates an HTML comparison report. Separate from the prism binary — it shells out to `prism review` and reads JSON output.

## Design

### Usage

```bash
# Build the benchmark tool:
go build -o benchmark ./cmd/benchmark

# Record golden files from the current binary:
./benchmark record --config benchmark.yml

# Compare current output against goldens:
./benchmark compare --config benchmark.yml

# Full before/after across two branches:
./benchmark run --config benchmark.yml --baseline main

# Generate HTML report from last run:
./benchmark report --dir results/benchmark
```

### Config File

```yaml
# benchmark.yml
prs:
  - repo: arinorr/prism
    number: 23
    description: "Go code-only (3 files)"
    tags: [go, code-only]
  - repo: arinorr/prism
    number: 14
    description: "Markdown + .tape (mixed docs)"
    tags: [mixed, docs]
  - repo: arinorr/prism
    number: 22
    description: "Go code (15 files, score refactoring)"
    tags: [go, large]
  - repo: arinorr/prism
    number: 18
    description: "Go code + tests (10 files)"
    tags: [go, mixed, tests]
  - repo: arinorr/ari-cloud
    number: 13
    description: "TypeScript monorepo (27 files)"
    tags: [typescript, large]
  - repo: arinorr/ari-cloud
    number: 9
    description: "TS + tests + middleware"
    tags: [typescript, mixed, tests]
  - repo: arinorr/whetstone
    number: 1
    description: "Go + YAML + Makefile + .md (all categories)"
    tags: [go, mixed, best-routing-test]
  - repo: arinorr/whetstone
    number: 3
    description: "Go + tests (20 files)"
    tags: [go, tests]

# Optional: default flags passed to prism
defaults:
  model: ""          # empty = use prism default
  verify: false
  timeout: "5m"

# Optional: filter by tags
# run_tags: [go, mixed]  # only run PRs matching these tags
```

### Directory Structure

```
cmd/benchmark/
  main.go          # CLI entry point, subcommands
  config.go        # YAML config parsing
  runner.go        # Executes prism binary, captures output
  compare.go       # Compares current vs golden, computes diffs
  report.go        # Generates HTML report
  report.html.tmpl # HTML template (embedded via go:embed)

testdata/benchmark/
  golden/           # Recorded baseline outputs (committed to git)
    arinorr-prism-pr-23.json
    arinorr-ari-cloud-pr-13.json
    ...
  benchmark.yml     # Default config (committed)
```

### Subcommands

#### `record` — Save Golden Files

Runs prism on each PR in the config, saves the JSON output as golden files. These are committed to git and serve as the expected baseline.

```bash
./benchmark record --config benchmark.yml [--prism ./prism] [--flags "--model haiku"]
```

Flow:
1. Read config
2. For each PR: run `prism review <URL> --format json --stdout --yes [extra flags]`
3. Save JSON to `testdata/benchmark/golden/{slug}.json`
4. Print summary: N PRs recorded, total cost

Golden files include a metadata header (timestamp, prism version, flags used) so you know when they were recorded:
```json
{
  "_benchmark_meta": {
    "recorded_at": "2026-04-08T10:00:00Z",
    "prism_flags": "--model haiku --yes",
    "prism_branch": "main"
  },
  "pr": { ... },
  "summary": "...",
  "deduped_findings": [ ... ],
  ...
}
```

#### `compare` — Compare Current vs Golden

Runs prism on each PR, then diffs the output against golden files. Reports regressions.

```bash
./benchmark compare --config benchmark.yml [--prism ./prism] [--flags "--verify"]
```

Flow:
1. Read config
2. For each PR: run prism, save to `testdata/benchmark/current/{slug}.json`
3. Load golden from `testdata/benchmark/golden/{slug}.json`
4. Compare: finding count, finding content, cost, score, dismissed/downgraded
5. Print diff summary + generate HTML report

Comparison logic:
```go
type Comparison struct {
    PR          PRConfig
    Golden      *PrismOutput   // may be nil if no golden exists
    Current     *PrismOutput
    Diff        ComparisonDiff
}

type ComparisonDiff struct {
    FindingsAdded    []Finding  // in current but not golden
    FindingsRemoved  []Finding  // in golden but not current
    FindingsChanged  []FindingDiff // same file+line, different risk/summary
    ScoreDelta       int        // current score - golden score
    CostDelta        float64    // current cost - golden cost
    DismissedCount   int        // from verifier (current only)
    DowngradedCount  int
    AgentsSkipped    []string   // agents that didn't run due to routing
}
```

Finding matching for diff: match by `file + line + category`. If a finding exists in both at the same location with the same category, compare risk and summary. If risk changed, it's a `FindingChanged`. If the finding only exists in one side, it's added or removed.

#### `run` — Full Before/After

Builds prism from two branches and runs both.

```bash
./benchmark run --config benchmark.yml --baseline main [--flags "--verify"]
```

Flow:
1. Record current branch name
2. Stash uncommitted changes
3. Checkout baseline branch, `go build -o /tmp/prism-baseline .`
4. Run all PRs with baseline binary → save to `results/benchmark/baseline/`
5. Checkout current branch, `go build -o /tmp/prism-current .`
6. Run all PRs with current binary → save to `results/benchmark/current/`
7. Restore branch + unstash
8. Compare baseline vs current (same logic as `compare` but between two run sets instead of current vs golden)
9. Generate HTML report

Error handling:
- Build failure → abort that phase, report which branch failed
- PR run failure → record the error, show "FAILED" in report, continue to next PR
- Timeout → kill after configurable duration (default 5 min per PR)

#### `report` — Generate HTML From Existing Data

Re-generates the HTML report from previously saved JSON files without re-running anything.

```bash
./benchmark report --dir results/benchmark --output benchmark.html
```

Useful when you want to tweak the report template without re-running $40 of LLM calls.

### HTML Report

Generated with Go's `html/template` (not bash heredocs). Embedded via `go:embed`.

Sections:
1. **Header** — baseline branch, current branch, timestamp, flags
2. **Summary table** — all PRs with key metrics and deltas
3. **Regression alerts** — red banner if any real findings were lost
4. **Per-PR cards** — expandable, showing:
   - Metric grid (findings, cost, score, tokens)
   - Δ indicators (green for improvement, red for regression)
   - Findings diff (added/removed/changed)
   - Routing log (from stderr capture)
   - Agents skipped
5. **Footer** — total cost, total time

### PrismOutput Struct

Parsed from the JSON output of `prism review --format json`:

```go
type PrismOutput struct {
    PR              PRSummary        `json:"pr"`
    Summary         string           `json:"summary"`
    HealthScore     HealthScore      `json:"health_score"`
    Findings        []Finding        `json:"findings"`
    DedupedFindings []DedupedFinding `json:"deduped_findings"`
    Usage           Usage            `json:"usage"`
    DismissedCount  int              `json:"dismissed_count"`
    DowngradedCount int              `json:"downgraded_count"`
    Duration        string           `json:"duration"`
}

type Finding struct {
    File     string  `json:"file"`
    Line     int     `json:"line"`
    Risk     string  `json:"risk"`
    Category string  `json:"category"`
    Summary  string  `json:"summary"`
}

// ... etc — mirrors prism's JSON output, no import dependency
```

These are **independent types** — no import from `internal/agents`. The benchmark tool reads JSON, it doesn't share types with prism. This keeps it truly standalone.

### Runner

```go
type Runner struct {
    PrismBinary string
    ExtraFlags  []string
    Timeout     time.Duration
}

type RunResult struct {
    PR       PRConfig
    Output   *PrismOutput  // nil if run failed
    Log      string        // stderr capture
    Error    error         // non-nil if run failed
    Duration time.Duration
    ExitCode int
}

func (r *Runner) Run(ctx context.Context, pr PRConfig) RunResult
```

The runner:
1. Builds the command: `prism review https://github.com/{repo}/pull/{number} --format json --stdout --yes {extra flags}`
2. Sets up stdout and stderr pipes
3. Runs with context (timeout via `context.WithTimeout`)
4. Captures stdout (JSON) and stderr (progress log)
5. Parses JSON into `PrismOutput`
6. Returns structured `RunResult`

### Tag Filtering

```bash
# Run only PRs tagged "go":
./benchmark compare --config benchmark.yml --tags go

# Run only mixed PRs:
./benchmark compare --config benchmark.yml --tags mixed

# Run specific PRs by number:
./benchmark compare --config benchmark.yml --prs 23,14
```

### Regression Detection

A regression is defined as:
- **Critical:** A finding present in golden with risk=critical or risk=warning is MISSING from current output. Real issue was lost.
- **Warning:** Health score dropped by more than 10 points.
- **Info:** Cost increased by more than 50%. Finding count changed significantly.

The `compare` subcommand exits with code 1 if any critical regressions are detected. This makes it usable in CI:

```yaml
# .github/workflows/ci.yml
- name: Benchmark regression check
  run: |
    go build -o benchmark ./cmd/benchmark
    ./benchmark compare --config benchmark.yml
```

## Files to Create

| File | Purpose |
|------|---------|
| `cmd/benchmark/main.go` | CLI entry point, subcommand dispatch |
| `cmd/benchmark/config.go` | YAML config parsing, PRConfig types |
| `cmd/benchmark/runner.go` | Execute prism binary, capture output, parse JSON |
| `cmd/benchmark/compare.go` | Diff current vs golden, regression detection |
| `cmd/benchmark/report.go` | HTML report generation from comparison data |
| `cmd/benchmark/report.html.tmpl` | Embedded HTML template |
| `testdata/benchmark/benchmark.yml` | Default config with 8 PRs |

## Files to Delete

| File | Reason |
|------|--------|
| `scripts/benchmark.sh` | Replaced by Go tool |

## Dependencies

- `gopkg.in/yaml.v3` — already in go.mod (used by config package)
- No other new dependencies

## Testing

- Unit: config parsing (valid, missing file, bad YAML)
- Unit: PrismOutput JSON parsing (valid, malformed, empty)
- Unit: comparison diff logic (added/removed/changed findings, score/cost deltas)
- Unit: regression detection (critical/warning/info thresholds)
- Unit: tag filtering
- Unit: slug generation from repo+number
- Integration: runner with a mock prism binary (shell script that outputs known JSON)
- Integration: full record → compare cycle with test fixtures

## Estimation

- ~400 lines: main.go + config.go + runner.go
- ~300 lines: compare.go + regression detection
- ~200 lines: report.go + HTML template
- ~200 lines: tests
- Total: ~1100 lines
- Replaces ~300 lines of bash

## Migration

1. Build the Go tool
2. Record golden files from current main: `./benchmark record`
3. Delete `scripts/benchmark.sh`
4. Update BACKLOG to reference the new tool
5. Commit golden files to git

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

# Validate config without running anything:
./benchmark compare --config benchmark.yml --dry-run

# Filter by tag:
./benchmark compare --config benchmark.yml --tags go,mixed
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

# Default flags passed to prism review.
defaults:
  model: ""
  verify: false
  timeout: "5m"

# Regression thresholds. These are INITIAL GUESSES — tune after collecting
# data on natural LLM variance across multiple runs.
thresholds:
  score_drop_critical: 10     # health score drop that triggers critical regression
  cost_increase_warning: 0.50 # 50% cost increase triggers warning
  finding_match_line_window: 10  # ±lines for fuzzy finding matching
```

### Directory Structure

```
cmd/benchmark/
  main.go              # CLI entry point, subcommand dispatch
  config.go            # YAML config parsing
  runner.go            # Executes prism binary, captures output
  compare.go           # Compares current vs golden, computes diffs
  report.go            # Generates HTML report
  report.html.tmpl     # HTML template (embedded via go:embed)

testdata/benchmark/
  golden/              # Recorded baseline outputs (committed to git)
    arinorr-prism-pr-23.golden.json
    arinorr-ari-cloud-pr-13.golden.json
    ...
  benchmark.yml        # Default config (committed)
```

### Subcommands

#### `record` — Save Golden Files

Runs prism on each PR, saves the JSON output as golden files committed to git.

```bash
./benchmark record --config benchmark.yml [--prism ./prism] [--flags "--model haiku"]
```

Golden files use a wrapper struct that cleanly separates metadata from prism output:

```go
// GoldenFile wraps prism's output with benchmark metadata.
// Meta and Output are separate concerns — no mutation of prism's format.
type GoldenFile struct {
    Meta   BenchmarkMeta `json:"meta"`
    Output PrismOutput   `json:"output"` // exactly what prism produces
}

type BenchmarkMeta struct {
    RecordedAt  time.Time `json:"recorded_at"`
    PrismFlags  string    `json:"prism_flags"`
    PrismBranch string    `json:"prism_branch"`
}
```

#### `compare` — Compare Current vs Golden

Runs prism on each PR, diffs against golden files. Reports regressions.

```bash
./benchmark compare --config benchmark.yml [--prism ./prism] [--flags "--verify"]
```

Exits with code 1 if critical regressions detected (CI-friendly).

#### `run` — Full Before/After

Builds prism from two branches using **git worktrees** (not checkout) and runs both.

```bash
./benchmark run --config benchmark.yml --baseline main [--flags "--verify"]
```

**Git worktree, not checkout.** The `run` command never touches the user's working directory. It creates temporary worktrees for each branch, builds there, and cleans up after:

```go
func buildFromBranch(branch string) (binaryPath string, cleanup func(), err error) {
    dir, err := os.MkdirTemp("", "benchmark-*")
    if err != nil {
        return "", nil, fmt.Errorf("creating temp dir: %w", err)
    }

    // git worktree add <dir> <branch> — isolated copy, no stash needed
    cmd := exec.Command("git", "worktree", "add", dir, branch)
    if err := cmd.Run(); err != nil {
        os.RemoveAll(dir)
        return "", nil, fmt.Errorf("creating worktree for %s: %w", branch, err)
    }

    // Build prism in the worktree.
    binary := filepath.Join(dir, "prism")
    buildCmd := exec.Command("go", "build", "-o", binary, ".")
    buildCmd.Dir = dir
    if err := buildCmd.Run(); err != nil {
        exec.Command("git", "worktree", "remove", dir, "--force").Run()
        return "", nil, fmt.Errorf("building prism from %s: %w", branch, err)
    }

    cleanup = func() {
        exec.Command("git", "worktree", "remove", dir, "--force").Run()
    }
    return binary, cleanup, nil
}
```

No stash, no checkout, no risk to user's working tree. The worktree is a separate directory that git manages. Cleaned up after each phase.

Error handling: build failure → abort that phase, report the error, continue with the other branch if possible.

#### `report` — Generate HTML From Existing Data

Re-generates the HTML report without re-running.

```bash
./benchmark report --dir results/benchmark --output benchmark.html
```

### PrismOutput Struct

Parses ALL fields from prism's JSON output. No imports from `internal/agents` — independent types that mirror the JSON structure:

```go
type PrismOutput struct {
    PR              PRSummary        `json:"pr"`
    Summary         string           `json:"summary"`
    HealthScore     HealthScore      `json:"health_score"`
    Findings        []Finding        `json:"findings"`
    DedupedFindings []DedupedFinding `json:"deduped_findings"`
    Suggestions     []Suggestion     `json:"suggestions"`
    Roles           []string         `json:"roles"`
    FailedAgents    []string         `json:"failed_agents"`
    Usage           Usage            `json:"usage"`
    Duration        string           `json:"duration"`
    DismissedCount  int              `json:"dismissed_count"`
    DowngradedCount int              `json:"downgraded_count"`
    VerifierError   string           `json:"verifier_error"`
}

type PRSummary struct {
    Number string `json:"number"`
    Title  string `json:"title"`
    Files  int    `json:"files"`
}

type HealthScore struct {
    Score       int    `json:"score"`
    Grade       string `json:"grade"`
    Verdict     string `json:"verdict"`
    Description string `json:"description"`
}

type Finding struct {
    File       string  `json:"file"`
    Line       int     `json:"line"`
    Risk       string  `json:"risk"`
    Category   string  `json:"category"`
    Scope      string  `json:"scope"`
    Confidence float64 `json:"confidence"`
    Summary    string  `json:"summary"`
    Detail     string  `json:"detail"`
}

type DedupedFinding struct {
    Finding
    VoteCount          int    `json:"vote_count"`
    TotalAgents        int    `json:"total_agents"`
    VerificationStatus string `json:"verification_status"`
    VerificationReason string `json:"verification_reason"`
}

type Usage struct {
    InputTokens  int     `json:"input_tokens"`
    OutputTokens int     `json:"output_tokens"`
    CostUSD      float64 `json:"cost_usd"`
    DurationMS   int     `json:"duration_ms"`
}

type Suggestion struct {
    File string `json:"file"`
    Line int    `json:"line"`
    Body string `json:"body"`
    Role string `json:"role"`
}
```

### Runner

```go
type Runner struct {
    PrismBinary string
    ExtraFlags  []string
    Timeout     time.Duration
}

// binary returns the prism binary path, defaulting to "prism" in PATH.
func (r *Runner) binary() string {
    if r.PrismBinary == "" {
        return "prism"
    }
    return r.PrismBinary
}

// timeout returns the per-PR timeout, defaulting to 5 minutes.
func (r *Runner) timeout() time.Duration {
    if r.Timeout == 0 {
        return 5 * time.Minute
    }
    return r.Timeout
}

type RunResult struct {
    PR       PRConfig
    Output   *PrismOutput  // nil if run failed
    Log      string        // stderr capture (progress messages)
    Error    error         // non-nil if run failed
    Duration time.Duration
    ExitCode int
}

func (r *Runner) Run(ctx context.Context, pr PRConfig) RunResult
```

PRs run **sequentially** by default. LLM rate limits, cost predictability, and deterministic log ordering all favor sequential execution. Parallel execution is a future option (`--parallel N`) for when speed matters more than predictability.

### Finding Matching — Fuzzy, Not Exact

LLM output is non-deterministic. The same PR might produce a finding at line 42 in one run and line 44 in another. Exact `file + line + category` matching produces phantom regressions.

**Matching strategy:**

```go
type FindingKey struct {
    File     string
    Category string
}

// MatchFindings pairs golden and current findings using fuzzy matching.
// Primary key: file + category (exact match).
// Secondary: nearest line within ±lineWindow.
// Tiebreaker: summary text similarity (Jaccard on tokens).
func MatchFindings(golden, current []DedupedFinding, lineWindow int) MatchResult

type MatchResult struct {
    Matched   []FindingPair  // paired: golden ↔ current
    Added     []DedupedFinding // in current only (new findings)
    Removed   []DedupedFinding // in golden only (regressions if critical/warning)
}

type FindingPair struct {
    Golden  DedupedFinding
    Current DedupedFinding
    Diff    FindingDiff
}

type FindingDiff struct {
    RiskChanged    bool   // e.g., warning → info
    SummaryChanged bool   // content changed
    LineDelta      int    // line number shift
}
```

Algorithm:
1. Group all findings by `FindingKey{File, Category}`
2. Within each group, match golden → current by nearest line (within `lineWindow`, default ±10)
3. For multiple candidates at similar lines, use summary token Jaccard similarity as tiebreaker
4. Unmatched golden findings → `Removed` (potential regressions)
5. Unmatched current findings → `Added` (new findings, may be good or bad)

### Regression Detection

```go
type RegressionLevel int
const (
    RegressionNone     RegressionLevel = iota
    RegressionInfo                     // informational change
    RegressionWarning                  // concerning but not blocking
    RegressionCritical                 // blocking — real findings lost
)

func ClassifyRegression(diff MatchResult, scoreDelta int, costDelta float64, thresholds Thresholds) RegressionLevel
```

**Regression levels:**

| Level | Trigger | Exit Code |
|-------|---------|-----------|
| Critical | A finding with risk=critical or risk=warning was REMOVED (present in golden, missing in current) | 1 |
| Warning | Health score dropped by more than `score_drop_critical` (default 10) OR cost increased by more than `cost_increase_warning` (default 50%) | 0 |
| Info | Finding count changed, findings added, minor risk changes | 0 |
| None | Output substantially unchanged | 0 |

**Only critical regressions cause exit code 1.** Everything else is informational. This is deliberate:
- LLM output varies between runs. Score fluctuations of ±5 points are normal.
- New findings being added is expected (especially after pipeline improvements).
- The thing we truly can't tolerate is a **real issue being silently dropped**.

**Thresholds are configurable** in the YAML and explicitly documented as initial guesses to tune after collecting data on natural variance.

### LLM Non-Determinism Strategy

Golden files for LLM output are fundamentally different from golden files for deterministic code. The same input can produce different outputs across runs.

**Approach:** Golden files represent "one known-good output," not "the correct output." The comparison reports **changes**, it doesn't **fail** on changes (except critical regressions). Deviations are expected and reviewed by a human.

**Practical implications:**
- `compare` shows a human-readable diff, not a pass/fail for every field
- Score deltas and finding count changes are informational, not failures
- Only "finding disappeared" is a critical regression (something got worse)
- Re-record goldens periodically (e.g., after each release) to track current behavior
- Future enhancement: store multiple golden runs and compare against ranges

### --dry-run

All subcommands support `--dry-run`:
- `record --dry-run`: parse config, validate PR access (`gh pr view --json number`), print what would run
- `compare --dry-run`: parse config, check golden files exist, print comparison plan
- `run --dry-run`: parse config, verify branches exist, print build + run plan

No LLM calls, no cost.

### HTML Report

Generated with Go's `html/template` + `go:embed`. Self-contained HTML file.

Sections:
1. **Header** — branches, timestamp, flags, regression level badge
2. **Summary table** — all PRs with metrics + deltas + regression indicators
3. **Regression banner** — red/yellow/green based on worst regression level
4. **Per-PR cards** (expandable):
   - Metric grid: findings, cost, score, tokens (with Δ)
   - Findings diff: added (green), removed (red), changed (yellow)
   - Failed agents (if any, explains missing findings)
   - Routing log (stderr capture)
   - Agents skipped (from routing)
5. **Footer** — total cost, total time

## Files to Create

| File | Purpose |
|------|---------|
| `cmd/benchmark/main.go` | CLI entry point, subcommand dispatch, --dry-run |
| `cmd/benchmark/config.go` | YAML config parsing, PRConfig, Thresholds |
| `cmd/benchmark/runner.go` | Execute prism, capture output, parse JSON, git worktree builds |
| `cmd/benchmark/compare.go` | Fuzzy finding matching, regression detection |
| `cmd/benchmark/report.go` | HTML report generation |
| `cmd/benchmark/report.html.tmpl` | Embedded HTML template |
| `cmd/benchmark/types.go` | PrismOutput, GoldenFile, all JSON types (independent, no prism imports) |
| `testdata/benchmark/benchmark.yml` | Default config with 8 PRs |

## Files to Delete

| File | Reason |
|------|--------|
| `scripts/benchmark.sh` | Replaced by Go tool |

## Dependencies

- `gopkg.in/yaml.v3` — already in go.mod
- No other new dependencies

## Testing

- Unit: config parsing (valid, missing file, bad YAML, tag filtering)
- Unit: PrismOutput JSON parsing (valid, malformed, empty, extra fields)
- Unit: fuzzy finding matching (exact match, line shift, no match, multiple candidates)
- Unit: regression detection (critical/warning/info/none thresholds)
- Unit: GoldenFile wrapper (meta + output separation)
- Unit: Runner defaults (zero-value binary and timeout)
- Unit: slug generation
- Integration: runner with a mock prism binary (script that outputs known JSON)
- Integration: full record → compare cycle with test fixtures
- Integration: git worktree build (requires a git repo)

## Estimation

- ~500 lines: main.go + config.go + runner.go + types.go
- ~350 lines: compare.go + fuzzy matching + regression detection
- ~250 lines: report.go + HTML template
- ~300 lines: tests
- Total: ~1400-1800 lines
- Replaces ~300 lines of bash

## Migration

1. Build the Go tool
2. Record golden files from current main: `./benchmark record`
3. Commit golden files to `testdata/benchmark/golden/`
4. Delete `scripts/benchmark.sh`
5. Update BACKLOG to reference the new tool

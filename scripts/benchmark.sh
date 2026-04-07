#!/usr/bin/env bash
set -euo pipefail

# benchmark.sh — Run Prism on a set of PRs across two branches and generate
# a side-by-side HTML comparison report.
#
# Usage:
#   ./scripts/benchmark.sh [--dry-run] [--model haiku] [--verify]
#
# The script:
#   1. Builds prism from the baseline branch (main)
#   2. Runs each PR and saves JSON results
#   3. Builds prism from the current branch
#   4. Runs each PR with routing + optional --verify
#   5. Generates an HTML comparison report
#
# Results are saved to: results/benchmark/

# --- Configuration ---

BASELINE_BRANCH="main"
CURRENT_BRANCH="$(git branch --show-current)"
RESULTS_DIR="results/benchmark"
REPORT_FILE="${RESULTS_DIR}/benchmark-report.html"
MODEL_FLAG=""
VERIFY_FLAG=""
DRY_RUN=""
EXTRA_FLAGS=""

# Test matrix: repo PR_number
MATRIX=(
    "arinorr/prism 23"
    "arinorr/prism 14"
    "arinorr/prism 22"
    "arinorr/prism 18"
    "arinorr/ari-cloud 13"
    "arinorr/ari-cloud 9"
    "arinorr/whetstone 1"
    "arinorr/whetstone 3"
)

# --- Parse Args ---

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)  DRY_RUN="true"; shift ;;
        --model)    MODEL_FLAG="--model $2"; shift 2 ;;
        --verify)   VERIFY_FLAG="--verify"; shift ;;
        *)          EXTRA_FLAGS="$EXTRA_FLAGS $1"; shift ;;
    esac
done

# --- Helpers ---

log() { echo "$(date '+%H:%M:%S') | $*"; }
err() { echo "$(date '+%H:%M:%S') | ERROR: $*" >&2; }

run_pr() {
    local repo="$1" pr="$2" binary="$3" label="$4" outdir="$5" extra="$6"
    local slug="${repo//\//-}-pr-${pr}"
    local json_file="${outdir}/${slug}.json"
    local log_file="${outdir}/${slug}.log"

    log "  [$label] ${repo}#${pr}..."

    if [[ -n "$DRY_RUN" ]]; then
        # Dry run: just check routing classification.
        "$binary" review "https://github.com/${repo}/pull/${pr}" \
            --dry-run --verbose --format json --stdout \
            $MODEL_FLAG $extra $EXTRA_FLAGS \
            > "$json_file" 2> "$log_file" || true
    else
        "$binary" review "https://github.com/${repo}/pull/${pr}" \
            --format json --stdout --yes \
            $MODEL_FLAG $extra $EXTRA_FLAGS \
            > "$json_file" 2> "$log_file" || true
    fi

    log "  [$label] ${repo}#${pr} → $(wc -c < "$json_file" | tr -d ' ') bytes"
}

build_prism() {
    local branch="$1" outbin="$2"
    log "Building prism from $branch..."
    git checkout "$branch" --quiet
    go build -o "$outbin" . 2>/dev/null
    log "Built $outbin"
}

# --- Main ---

log "Benchmark: $BASELINE_BRANCH vs $CURRENT_BRANCH"
log "PRs: ${#MATRIX[@]}"
log "Model: ${MODEL_FLAG:-default}"
log "Verify: ${VERIFY_FLAG:-no}"
log "Dry run: ${DRY_RUN:-no}"
echo

mkdir -p "${RESULTS_DIR}/baseline" "${RESULTS_DIR}/current"

# Stash any uncommitted changes.
STASHED=""
if ! git diff --quiet 2>/dev/null; then
    git stash --quiet
    STASHED="true"
fi

# Phase 1: Build and run baseline.
log "=== Phase 1: Baseline ($BASELINE_BRANCH) ==="
build_prism "$BASELINE_BRANCH" "${RESULTS_DIR}/prism-baseline"

for entry in "${MATRIX[@]}"; do
    read -r repo pr <<< "$entry"
    run_pr "$repo" "$pr" "${RESULTS_DIR}/prism-baseline" "baseline" "${RESULTS_DIR}/baseline" ""
done

# Phase 2: Build and run current branch.
log ""
log "=== Phase 2: Current ($CURRENT_BRANCH) ==="
build_prism "$CURRENT_BRANCH" "${RESULTS_DIR}/prism-current"

for entry in "${MATRIX[@]}"; do
    read -r repo pr <<< "$entry"
    run_pr "$repo" "$pr" "${RESULTS_DIR}/prism-current" "current" "${RESULTS_DIR}/current" "$VERIFY_FLAG"
done

# Restore original branch.
git checkout "$CURRENT_BRANCH" --quiet
if [[ -n "$STASHED" ]]; then
    git stash pop --quiet
fi

# Phase 3: Generate HTML report.
log ""
log "=== Phase 3: Generating report ==="

generate_report() {
    cat <<'HTMLEOF'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Prism Benchmark Report</title>
<style>
:root { --bg: #fff; --fg: #1f2328; --muted: #656d76; --border: #d0d7de; --surface: #f6f8fa;
  --green: #1a7f37; --green-bg: #dafbe1; --red: #cf222e; --red-bg: #ffebe9;
  --blue: #0969da; --blue-bg: #ddf4ff; --yellow: #9a6700; --yellow-bg: #fff8c5; }
* { box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, sans-serif;
  max-width: 1100px; margin: 0 auto; padding: 2rem; color: var(--fg); line-height: 1.6; }
h1 { font-size: 1.5rem; }
h2 { font-size: 1.2rem; margin-top: 2rem; border-bottom: 1px solid var(--border); padding-bottom: 0.4rem; }
table { width: 100%; border-collapse: collapse; margin: 1rem 0; font-size: 0.85rem; }
th, td { padding: 0.5rem 0.75rem; border: 1px solid var(--border); text-align: left; }
th { background: var(--surface); font-weight: 600; }
tr:hover { background: var(--surface); }
.good { color: var(--green); font-weight: 600; }
.bad { color: var(--red); font-weight: 600; }
.neutral { color: var(--muted); }
.card { border: 1px solid var(--border); border-radius: 8px; margin: 1rem 0; overflow: hidden; }
.card-header { background: var(--surface); padding: 0.75rem 1rem; font-weight: 600; border-bottom: 1px solid var(--border);
  display: flex; justify-content: space-between; }
.card-body { padding: 1rem; }
.badge { font-size: 0.7rem; padding: 0.15rem 0.5rem; border-radius: 10px; font-weight: 700; }
.badge-green { background: var(--green-bg); color: var(--green); }
.badge-red { background: var(--red-bg); color: var(--red); }
.badge-blue { background: var(--blue-bg); color: var(--blue); }
.badge-yellow { background: var(--yellow-bg); color: var(--yellow); }
.metric-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 0.75rem; margin: 0.75rem 0; }
.metric { background: var(--surface); border-radius: 6px; padding: 0.75rem; text-align: center; }
.metric-value { font-size: 1.2rem; font-weight: 700; }
.metric-label { font-size: 0.75rem; color: var(--muted); }
.delta { font-size: 0.8rem; }
.routing-log { font-family: monospace; font-size: 0.8rem; background: var(--surface); padding: 0.75rem;
  border-radius: 6px; max-height: 200px; overflow-y: auto; white-space: pre-wrap; }
.footer { text-align: center; color: var(--muted); font-size: 0.8rem; margin-top: 2rem; border-top: 1px solid var(--border); padding-top: 1rem; }
</style>
</head>
<body>
HTMLEOF

    # Header.
    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M')"
    cat <<EOF
<h1>Prism Benchmark Report</h1>
<p>Baseline: <code>$BASELINE_BRANCH</code> vs Current: <code>$CURRENT_BRANCH</code></p>
<p>Generated: ${timestamp} | Model: ${MODEL_FLAG:-default} | Verify: ${VERIFY_FLAG:-disabled}</p>

<h2>Summary</h2>
<table>
<tr>
  <th>PR</th>
  <th>Repo</th>
  <th>Baseline Findings</th>
  <th>Current Findings</th>
  <th>Baseline Cost</th>
  <th>Current Cost</th>
  <th>Δ Cost</th>
  <th>Dismissed</th>
  <th>Downgraded</th>
</tr>
EOF

    # Per-PR rows.
    for entry in "${MATRIX[@]}"; do
        read -r repo pr <<< "$entry"
        local slug="${repo//\//-}-pr-${pr}"
        local base_json="${RESULTS_DIR}/baseline/${slug}.json"
        local curr_json="${RESULTS_DIR}/current/${slug}.json"

        # Extract metrics from JSON (jq if available, else defaults).
        local base_findings="-" curr_findings="-" base_cost="-" curr_cost="-"
        local delta_cost="-" dismissed="-" downgraded="-"

        if command -v jq &>/dev/null; then
            if [[ -s "$base_json" ]]; then
                base_findings=$(jq '.deduped_findings | length // .findings | length // 0' "$base_json" 2>/dev/null || echo "0")
                base_cost=$(jq '.usage.cost_usd // 0' "$base_json" 2>/dev/null || echo "0")
            fi
            if [[ -s "$curr_json" ]]; then
                curr_findings=$(jq '.deduped_findings | length // .findings | length // 0' "$curr_json" 2>/dev/null || echo "0")
                curr_cost=$(jq '.usage.cost_usd // 0' "$curr_json" 2>/dev/null || echo "0")
                dismissed=$(jq '.dismissed_count // 0' "$curr_json" 2>/dev/null || echo "0")
                downgraded=$(jq '.downgraded_count // 0' "$curr_json" 2>/dev/null || echo "0")
            fi

            if [[ "$base_cost" != "-" && "$curr_cost" != "-" ]]; then
                delta_cost=$(echo "$base_cost $curr_cost" | awk '{printf "%.2f", $2 - $1}')
            fi
        fi

        local delta_class="neutral"
        if [[ "$delta_cost" != "-" ]]; then
            if (( $(echo "$delta_cost < 0" | bc -l 2>/dev/null || echo 0) )); then
                delta_class="good"
            elif (( $(echo "$delta_cost > 0" | bc -l 2>/dev/null || echo 0) )); then
                delta_class="bad"
            fi
        fi

        cat <<EOF
<tr>
  <td><a href="https://github.com/${repo}/pull/${pr}">#${pr}</a></td>
  <td>${repo}</td>
  <td>${base_findings}</td>
  <td>${curr_findings}</td>
  <td>\$${base_cost}</td>
  <td>\$${curr_cost}</td>
  <td class="${delta_class}">\$${delta_cost}</td>
  <td>${dismissed}</td>
  <td>${downgraded}</td>
</tr>
EOF
    done

    echo "</table>"

    # Per-PR detail cards.
    echo "<h2>Per-PR Details</h2>"

    for entry in "${MATRIX[@]}"; do
        read -r repo pr <<< "$entry"
        local slug="${repo//\//-}-pr-${pr}"
        local base_log="${RESULTS_DIR}/baseline/${slug}.log"
        local curr_log="${RESULTS_DIR}/current/${slug}.log"

        cat <<EOF
<div class="card">
  <div class="card-header">
    <span>${repo}#${pr}</span>
    <a href="https://github.com/${repo}/pull/${pr}" style="color: var(--blue); text-decoration: none;">View PR →</a>
  </div>
  <div class="card-body">
    <h3 style="margin-top:0">Routing Log (current branch)</h3>
    <div class="routing-log">$(cat "$curr_log" 2>/dev/null | head -30 || echo "No log available")</div>

    <h3>Baseline Log</h3>
    <div class="routing-log">$(cat "$base_log" 2>/dev/null | head -20 || echo "No log available")</div>
  </div>
</div>
EOF
    done

    cat <<'HTMLEOF'
<div class="footer">Generated by <a href="https://github.com/arinorr/prism">Prism</a> benchmark</div>
</body>
</html>
HTMLEOF
}

generate_report > "$REPORT_FILE"

log "Report written to $REPORT_FILE"
log "Done."

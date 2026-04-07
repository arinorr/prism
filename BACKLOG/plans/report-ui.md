# Report UI Improvements

## Context

The HTML report currently renders all findings in a single scrollable page, grouped by scope (Changed / Existing / Codebase) then by file. When a PR has many findings across severity levels, it's hard to focus on what matters — you have to scroll past info findings to find the criticals.

## Goal

Add a severity tab bar to the findings section. Each tab filters findings by severity (Critical, Warning, Info) while keeping the by-file grouping within each tab. When there are zero findings, show a clean empty state instead of tabs.

## Design

### Tab Bar

Rendered as a horizontal bar above the findings area with counts:

```
[Critical (3)] [Warning (8)] [Info (12)]
```

- Pure CSS + vanilla JS (no framework). The report is a single self-contained HTML file.
- Each tab shows/hides its content panel. Only one panel visible at a time.
- Active tab gets a bottom border highlight + bold text.

### Default Tab Selection

Smart default — open the most severe non-empty tab:
1. If critical findings exist → open Critical tab
2. Else if warning findings exist → open Warning tab  
3. Else → open Info tab

### Empty State

When there are **zero total findings** (no criticals, no warnings, no info), replace the entire tab container with:

```html
<div class="empty-state">
  <div class="empty-icon">✅</div>
  <div class="empty-title">All clear</div>
  <div class="empty-sub">No issues found in PR #{{.PRNumber}}</div>
</div>
```

Centered, clean, with some vertical breathing room. The health gauge above it already shows the score, so the empty state just confirms "nothing to act on."

### Content Within Tabs

Each tab panel contains the same by-file group structure that exists today, but filtered to only include findings of that severity. The scope sections (Changed / Existing / Codebase) are preserved within each tab.

Structure:

```
[Critical (3)] [Warning (8)] [Info (12)]

── Critical tab ──────────────────────
  Issues in this PR (2)
    handler.go
      ├─ SQL injection vulnerability
      └─ Missing auth check
  Pre-existing Issues (1)  
    db.go
      └─ Unparameterized query

── Warning tab (hidden) ──────────────
  Issues in this PR (6)
    ...
```

### JavaScript

Minimal — just tab switching:

```javascript
function showTab(severity) {
  document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  document.querySelectorAll('.tab-panel').forEach(p => p.style.display = 'none');
  document.querySelector(`[data-tab="${severity}"]`).classList.add('active');
  document.querySelector(`#panel-${severity}`).style.display = 'block';
}
```

No build step, no external dependencies. Inlined in a `<script>` tag at the bottom of the template.

### CSS Additions

```css
/* Tab bar */
.tab-bar { display: flex; gap: 0; border-bottom: 2px solid var(--border); margin: 1.5rem 0 0; }
.tab-btn { padding: 0.6rem 1.2rem; cursor: pointer; font-weight: 600; font-size: 0.85rem;
           border: none; background: none; color: var(--muted); border-bottom: 2px solid transparent;
           margin-bottom: -2px; transition: color 0.15s, border-color 0.15s; }
.tab-btn:hover { color: var(--fg); }
.tab-btn.active { color: var(--fg); border-bottom-color: var(--blue); }
.tab-btn .count { font-weight: 400; color: var(--muted); margin-left: 0.25rem; }
.tab-panel { display: none; }
.tab-panel.active { display: block; }

/* Empty state */
.empty-state { text-align: center; padding: 3rem 1rem; }
.empty-icon { font-size: 3rem; margin-bottom: 0.5rem; }
.empty-title { font-size: 1.25rem; font-weight: 700; margin-bottom: 0.25rem; }
.empty-sub { font-size: 0.9rem; color: var(--muted); }
```

### Template Data Changes

Add to `htmlTemplateData`:

```go
HasFindings  bool // true if any findings exist
```

The existing `CriticalCount`, `WarningCount`, `InfoCount` already provide what we need for tab counts and default selection.

We already have `ChangedFindings`, `ExistingFindings`, `CodebaseFindings` split by scope. For tabs, we need these split **by severity first, then by scope**. Add:

```go
CriticalChangedFindings  []htmlFileGroup
CriticalExistingFindings []htmlFileGroup
CriticalCodebaseFindings []htmlFileGroup
WarningChangedFindings   []htmlFileGroup
WarningExistingFindings  []htmlFileGroup
WarningCodebaseFindings  []htmlFileGroup
InfoChangedFindings      []htmlFileGroup
InfoExistingFindings     []htmlFileGroup
InfoCodebaseFindings     []htmlFileGroup
```

These are built by filtering the existing file groups by severity. Add a helper:

```go
func filterFileGroupsBySeverity(groups []htmlFileGroup, risk agents.Risk) []htmlFileGroup
```

This filters each group's findings to only include the given severity, then drops groups that become empty.

### Template Structure

```html
{{if .HasFindings}}
<div class="tab-bar">
  <button class="tab-btn{{if gt .CriticalCount 0}} active{{end}}" data-tab="critical"
          onclick="showTab('critical')">Critical <span class="count">({{.CriticalCount}})</span></button>
  <button class="tab-btn{{if and (eq .CriticalCount 0) (gt .WarningCount 0)}} active{{end}}" data-tab="warning"
          onclick="showTab('warning')">Warning <span class="count">({{.WarningCount}})</span></button>
  <button class="tab-btn{{if and (eq .CriticalCount 0) (eq .WarningCount 0)}} active{{end}}" data-tab="info"
          onclick="showTab('info')">Info <span class="count">({{.InfoCount}})</span></button>
</div>

<div id="panel-critical" class="tab-panel{{if gt .CriticalCount 0}} active{{end}}">
  {{if .CriticalChangedFindings}}...scope sections with filegroup template...{{end}}
  {{if eq .CriticalCount 0}}<p class="empty-tab">No critical findings.</p>{{end}}
</div>

<div id="panel-warning" class="tab-panel{{if and (eq .CriticalCount 0) (gt .WarningCount 0)}} active{{end}}">
  ...
</div>

<div id="panel-info" class="tab-panel{{if and (eq .CriticalCount 0) (eq .WarningCount 0)}} active{{end}}">
  ...
</div>

{{else}}
<div class="empty-state">
  <div class="empty-icon">✅</div>
  <div class="empty-title">All clear</div>
  <div class="empty-sub">No issues found in PR #{{.PRNumber}}</div>
</div>
{{end}}
```

## Files to Modify

| File | Change |
|------|--------|
| `internal/report/html.go` | Add `HasFindings`, severity-filtered file groups to template data. Add `filterFileGroupsBySeverity` helper. Update `HTML()` to populate new fields. |
| `internal/report/html.go` (template) | Replace scope-section blocks with tab bar + tab panels + empty state. Add CSS for tabs and empty state. Add `<script>` for tab switching. |
| `internal/report/data.go` | Add `filterFileGroupsBySeverity()` helper function |

## Files Unchanged

- `markdown.go` — tabs don't make sense in markdown, keep as-is
- `json.go` — structured data, no UI

## Testing

- Unit: `filterFileGroupsBySeverity` correctly filters and drops empty groups
- Unit: `HasFindings` is false when no findings
- Unit: HTML output contains tab buttons with correct counts
- Unit: HTML output contains empty state when zero findings
- Unit: default active tab follows the priority rule (critical > warning > info)
- Visual: manually open generated HTML report and verify tab switching works

## Edge Cases

- All findings are info → Info tab is active, Critical and Warning tabs show count (0)
- Single finding → tabs still render (consistent UI)
- Zero findings → empty state, no tabs
- Finding counts update correctly after verification dismisses some

## Estimation

- ~100 lines of template changes (CSS + HTML + JS)
- ~40 lines of Go changes (new fields, filter helper, population)
- No new dependencies

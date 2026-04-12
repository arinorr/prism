package report

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"

	"github.com/arinorr/prism/internal/agents"
)

// htmlTemplateData is the structured data passed to the HTML template.
type htmlTemplateData struct {
	PRNumber         string
	PRTitle          string
	FileCount        int
	AgentCount       int
	FindingCount     int
	Duration         string
	CriticalCount    int
	WarningCount     int
	InfoCount        int
	ChangedCount     int
	ExistingCount    int
	CodebaseCount    int
	SuggestionCount  int
	SummaryHTML      htmltemplate.HTML
	FileGroups       []htmlFileGroup
	ChangedFindings  []htmlFileGroup
	ExistingFindings []htmlFileGroup
	CodebaseFindings []htmlFileGroup
	FailedAgents     []string
	HealthScore      agents.HealthScore
	NeedleRotation   int    // SVG rotation angle for gauge needle (-90=left, 0=up, +90=right)
	GaugeColor       string // hex color for the gauge arc based on score
	GradeColor       string // hex color for the grade letter
	GaugeDashOffset  int    // SVG stroke-dashoffset for arc fill (0=full, 251=empty)
	HasUsage         bool
	TotalTokensK     int
	InputTokensK     int
	OutputTokensK    int
	CostUSD          string
	DismissedCount   int
	DowngradedCount  int
	HasVerification  bool
	VerifierError    string
}

type htmlFileGroup struct {
	File          string
	CriticalCount int
	WarningCount  int
	InfoCount     int
	Findings      []htmlFinding
}

type htmlFinding struct {
	Risk                agents.Risk
	RiskClass           string
	Line                int
	HasLine             bool
	Role                string
	RoleClass           string
	Summary             string
	Detail              string
	HasDetail           bool
	Category            string
	CategoryClass       string
	CodeExample         string
	HasCodeExample      bool
	Scope               string
	AgentDetails        []htmlAgentDetail
	HasMultipleAgents   bool
	FindingIndex        int
	VoteCount           int
	TotalAgents         int
	ConsensusPercent    int
	VerificationStatus  string
	VerificationReason  string
	HasVerificationInfo bool
}

type htmlAgentDetail struct {
	Role           string
	RoleClass      string
	Risk           string // this agent's individual severity opinion
	Detail         string
	CodeExample    string
	HasCodeExample bool
}

// agentColors maps role slugs to CSS color classes for badges.
var agentColors = map[string]string{
	"know-it-all":   "agent-purple",
	"architect":     "agent-indigo",
	"solver":        "agent-teal",
	"editor":        "agent-orange",
	"optimizer":     "agent-green",
	"sentinel":      "agent-red",
	"test-engineer": "agent-blue",
}

func agentColorClass(role string) string {
	if c, ok := agentColors[role]; ok {
		return c
	}
	return "agent-default"
}

// categoryClasses maps category slugs to CSS classes for badges.
var categoryClasses = map[string]string{
	"bug":         "cat-bug",
	"security":    "cat-security",
	"design":      "cat-design",
	"performance": "cat-perf",
	"style":       "cat-style",
	"testing":     "cat-test",
}

func categoryClass(cat string) string {
	if c, ok := categoryClasses[cat]; ok {
		return c
	}
	return "cat-design"
}

// HTML generates a styled HTML report using Go's html/template.
// Security: html/template auto-escapes all template variables, so
// untrusted LLM-generated content (Summary, Detail, CodeExample) is
// rendered safely without manual sanitization. Do not switch to
// text/template without adding explicit escaping.
func HTML(d *Data) (string, error) {
	// Convert synthesis summary from markdown to sanitized HTML.
	var rawHTML bytes.Buffer
	if err := goldmark.Convert([]byte(d.Result.Summary), &rawHTML); err != nil {
		return "", fmt.Errorf("markdown to HTML conversion failed: %w", err)
	}
	sanitizer := bluemonday.UGCPolicy()
	summaryHTML := sanitizer.Sanitize(rawHTML.String())

	// Build template data — prefer deduped findings when available.
	var critCount, warnCount, infoCount int
	var changedCount, existingCount, codebaseCount int
	var findingCount int
	var fileGroups []htmlFileGroup
	findingIndex := 0

	if len(d.Result.DedupedFindings) > 0 {
		findingCount = len(d.Result.DedupedFindings)
		for i := range d.Result.DedupedFindings {
			switch d.Result.DedupedFindings[i].Risk {
			case severityCritical:
				critCount++
			case severityWarning:
				warnCount++
			default:
				infoCount++
			}
			switch d.Result.DedupedFindings[i].Scope {
			case agents.ScopeChanged:
				changedCount++
			case agents.ScopeExisting:
				existingCount++
			default:
				codebaseCount++
			}
		}
		for _, g := range groupDedupedByFile(d.Result.DedupedFindings) {
			fg := buildDedupedFileGroup(g, &findingIndex)
			fileGroups = append(fileGroups, fg)
		}
	} else {
		findingCount = len(d.Result.Findings)
		for i := range d.Result.Findings {
			f := &d.Result.Findings[i]
			switch f.Risk {
			case severityCritical:
				critCount++
			case severityWarning:
				warnCount++
			default:
				infoCount++
			}
			switch f.Scope {
			case agents.ScopeChanged:
				changedCount++
			case agents.ScopeExisting:
				existingCount++
			default:
				codebaseCount++
			}
		}
		for _, g := range groupByFile(d.Result.Findings) {
			fg := buildRawFileGroup(g, &findingIndex)
			fileGroups = append(fileGroups, fg)
		}
	}

	// Split file groups by scope.
	changedFindings, existingFindings, codebaseFindings := splitByScope(fileGroups)

	td := htmlTemplateData{
		PRNumber:         d.PR.Number,
		PRTitle:          d.PR.Title,
		FileCount:        len(d.PR.Files),
		AgentCount:       len(d.Roles),
		FindingCount:     findingCount,
		Duration:         d.Duration,
		CriticalCount:    critCount,
		WarningCount:     warnCount,
		InfoCount:        infoCount,
		ChangedCount:     changedCount,
		ExistingCount:    existingCount,
		CodebaseCount:    codebaseCount,
		SuggestionCount:  len(d.Result.Suggestions),
		SummaryHTML:      htmltemplate.HTML(summaryHTML), // #nosec G203 -- already sanitized by bluemonday
		FileGroups:       fileGroups,
		ChangedFindings:  changedFindings,
		ExistingFindings: existingFindings,
		CodebaseFindings: codebaseFindings,
		FailedAgents:     d.Result.FailedAgents,
		HealthScore:      d.Result.HealthScore,
		NeedleRotation:   int(float64(d.Result.HealthScore.Score)*1.8) - 90, // 0->-90 (left), 50->0 (up), 100->+90 (right)
		GaugeColor:       gaugeColor(d.Result.HealthScore.Score),
		GradeColor:       gradeColor(d.Result.HealthScore.Score),
		GaugeDashOffset:  251 - (d.Result.HealthScore.Score*251)/100, // 251 ~ pi*80, the semicircle arc length
		HasUsage:         d.Usage.TotalTokens() > 0,
		InputTokensK:     d.Usage.TotalInputTokens() / 1000,
		OutputTokensK:    d.Usage.OutputTokens / 1000,
		TotalTokensK:     d.Usage.TotalTokens() / 1000,
		CostUSD:          fmt.Sprintf("%.2f", d.Usage.CostUSD),
		DismissedCount:   d.DismissedCount,
		DowngradedCount:  d.DowngradedCount,
		HasVerification:  d.DismissedCount > 0 || d.DowngradedCount > 0,
		VerifierError:    d.VerifierError,
	}

	tmpl, err := htmltemplate.New("report").Parse(htmlReportTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	return buf.String(), nil
}

func riskClass(risk agents.Risk) string {
	switch risk {
	case severityCritical:
		return "badge-critical"
	case severityWarning:
		return "badge-warning"
	default:
		return "badge-info"
	}
}

func buildDedupedFileGroup(g dedupedFileGroup, idx *int) htmlFileGroup {
	var fc, fw, fi int
	var findings []htmlFinding
	for i := range g.findings {
		f := &g.findings[i]
		switch f.Risk {
		case severityCritical:
			fc++
		case severityWarning:
			fw++
		default:
			fi++
		}

		var agentDetails []htmlAgentDetail
		for _, ad := range f.AgentDetails {
			agentDetails = append(agentDetails, htmlAgentDetail{
				Role:           ad.Role,
				RoleClass:      agentColorClass(ad.Role),
				Risk:           string(ad.Risk),
				Detail:         ad.Detail,
				CodeExample:    ad.CodeExample,
				HasCodeExample: ad.CodeExample != "",
			})
		}

		findings = append(findings, htmlFinding{
			Risk:                f.Risk,
			RiskClass:           riskClass(f.Risk),
			Line:                f.Line,
			HasLine:             f.Line > 0,
			Role:                f.Role,
			RoleClass:           agentColorClass(f.Role),
			Summary:             f.Summary,
			Detail:              f.Detail,
			HasDetail:           f.Detail != "",
			Category:            f.Category,
			CategoryClass:       categoryClass(f.Category),
			CodeExample:         f.CodeExample,
			HasCodeExample:      f.CodeExample != "",
			Scope:               f.Scope,
			AgentDetails:        agentDetails,
			HasMultipleAgents:   len(agentDetails) > 1,
			FindingIndex:        *idx,
			VoteCount:           f.VoteCount,
			TotalAgents:         f.TotalAgents,
			ConsensusPercent:    int(f.Consensus() * 100),
			VerificationStatus:  string(f.VerificationStatus),
			VerificationReason:  f.VerificationReason,
			HasVerificationInfo: f.VerificationStatus != "",
		})
		*idx++
	}
	return htmlFileGroup{
		File:          g.file,
		CriticalCount: fc,
		WarningCount:  fw,
		InfoCount:     fi,
		Findings:      findings,
	}
}

func buildRawFileGroup(g fileGroup, idx *int) htmlFileGroup {
	var fc, fw, fi int
	var findings []htmlFinding
	for j := range g.findings {
		f := &g.findings[j]
		switch f.Risk {
		case severityCritical:
			fc++
		case severityWarning:
			fw++
		default:
			fi++
		}
		findings = append(findings, htmlFinding{
			Risk:           f.Risk,
			RiskClass:      riskClass(f.Risk),
			Line:           f.Line,
			HasLine:        f.Line > 0,
			Role:           f.Role,
			RoleClass:      agentColorClass(f.Role),
			Summary:        f.Summary,
			Detail:         f.Detail,
			HasDetail:      f.Detail != "",
			Category:       f.Category,
			CategoryClass:  categoryClass(f.Category),
			CodeExample:    f.CodeExample,
			HasCodeExample: f.CodeExample != "",
			Scope:          f.Scope,
			FindingIndex:   *idx,
		})
		*idx++
	}
	return htmlFileGroup{
		File:          g.file,
		CriticalCount: fc,
		WarningCount:  fw,
		InfoCount:     fi,
		Findings:      findings,
	}
}

// gaugeColor returns a hex color for the health gauge arc based on score.
func gaugeColor(score int) string {
	switch {
	case score >= 80:
		return "#1a7f37" // green
	case score >= 60:
		return "#d4a72c" // yellow
	default:
		return "#cf222e" // red
	}
}

// gradeColor returns a hex color for the grade letter based on score.
func gradeColor(score int) string {
	switch {
	case score >= 70:
		return "#1a7f37" // green for A+, A, B+, B
	case score >= 60:
		return "#d4a72c" // yellow for C
	default:
		return "#cf222e" // red for D, F
	}
}

const htmlReportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Prism Review: PR #{{.PRNumber}}</title>
<style>
:root {
  --bg: #ffffff; --fg: #1f2328; --muted: #656d76; --border: #d0d7de;
  --surface: #f6f8fa;
  --red: #cf222e; --red-bg: #ffebe9;
  --yellow: #9a6700; --yellow-bg: #fff8c5;
  --blue: #0969da; --blue-bg: #ddf4ff;
  --green: #1a7f37; --green-bg: #dafbe1;
  --purple: #8250df; --purple-bg: #fbefff;
  --teal: #0d9488; --teal-bg: #e6fffa;
}
* { box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; max-width: 960px; margin: 0 auto; padding: 2rem 1.5rem; line-height: 1.6; color: var(--fg); background: var(--bg); }
h1 { font-size: 1.5rem; margin: 0; }
h2 { font-size: 1.25rem; margin: 2rem 0 1rem; padding-bottom: 0.4em; border-bottom: 1px solid var(--border); }

/* Header */
.header { border-bottom: 2px solid var(--border); padding-bottom: 1rem; margin-bottom: 1.5rem; }
.header .pr-title { color: var(--muted); font-size: 1rem; margin: 0.25rem 0 0.75rem; }
.meta { display: flex; gap: 1.5rem; flex-wrap: wrap; font-size: 0.85rem; color: var(--muted); }

/* Health gauge */
.gauge-container { text-align: center; margin: 1.5rem 0; }
.gauge-label { font-size: 0.9rem; color: var(--muted); margin-top: 0.25rem; }
.gauge-verdict { font-weight: 600; font-size: 1rem; margin-top: 0.25rem; }

/* Dashboard */
.dashboard { margin: 1.5rem 0; }
.dash-card { background: var(--surface); border: 1px solid var(--border); border-radius: 10px; padding: 1.25rem; }
.dash-card-wide { width: 100%; }
.dash-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.75rem; }
.dash-risks { display: flex; gap: 0.5rem; flex-wrap: wrap; }
.dash-badge { font-size: 0.75rem; font-weight: 700; padding: 0.25rem 0.7rem; border-radius: 10px; text-transform: uppercase; letter-spacing: 0.03em; }
.dash-verdict { font-size: 1.15rem; font-weight: 700; color: var(--fg); margin-bottom: 0.25rem; text-transform: capitalize; }
.dash-sub { font-size: 0.85rem; color: var(--muted); }

/* Failed agents banner */
.failed-banner { background: var(--yellow-bg); color: var(--yellow); padding: 0.75rem 1rem; border-radius: 6px; margin-bottom: 1rem; font-weight: 600; }

/* File groups */
.file-group { border: 1px solid var(--border); border-radius: 8px; margin: 1rem 0; overflow: hidden; }
.file-header { background: var(--surface); padding: 0.6rem 1rem; font-weight: 600; font-family: SFMono-Regular, Consolas, monospace; font-size: 0.9rem; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
.file-header .counts { display: flex; gap: 0.5rem; font-size: 0.75rem; font-weight: normal; font-family: -apple-system, sans-serif; }
.file-header .counts span { padding: 0.15rem 0.5rem; border-radius: 10px; }
.badge-critical { background: var(--red-bg); color: var(--red); }
.badge-warning { background: var(--yellow-bg); color: var(--yellow); }
.badge-info { background: var(--blue-bg); color: var(--blue); }

/* Category badges */
.cat-badge { font-size: 0.65rem; font-weight: 600; padding: 0.1rem 0.5rem; border-radius: 10px; text-transform: uppercase; letter-spacing: 0.03em; }
.cat-bug { background: var(--red-bg); color: var(--red); }
.cat-security { background: #fef2f2; color: #b91c1c; }
.cat-design { background: var(--blue-bg); color: var(--blue); }
.cat-perf { background: var(--green-bg); color: var(--green); }
.cat-style { background: var(--purple-bg); color: var(--purple); }
.cat-test { background: var(--teal-bg); color: var(--teal); }

/* Finding cards using details/summary */
details.finding-card { border-bottom: 1px solid var(--border); }
details.finding-card:last-child { border-bottom: none; }
details.finding-card > summary { padding: 0.75rem 1rem; cursor: pointer; display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; list-style: none; }
details.finding-card > summary::-webkit-details-marker { display: none; }
details.finding-card > summary::before { content: "\25b6"; font-size: 0.6rem; color: var(--muted); transition: transform 0.15s; }
details.finding-card[open] > summary::before { transform: rotate(90deg); }
details.finding-card > summary:hover { background: var(--surface); }
details.finding-card > summary .severity { font-size: 0.7rem; font-weight: 700; padding: 0.15rem 0.6rem; border-radius: 10px; text-transform: uppercase; letter-spacing: 0.04em; }
details.finding-card > summary .line { font-size: 0.8rem; color: var(--muted); font-family: SFMono-Regular, Consolas, monospace; }
details.finding-card > summary .finding-text { flex: 1; font-weight: 600; }
.finding-body { padding: 0.5rem 1rem 1rem 2rem; }
.finding-body .overview { font-size: 0.9rem; color: var(--muted); line-height: 1.5; margin-bottom: 0.75rem; }

/* Code example blocks */
pre.code-example { background: var(--surface); border: 1px solid var(--border); padding: 0.75rem; border-radius: 6px; overflow-x: auto; font-size: 0.8rem; font-family: SFMono-Regular, Consolas, monospace; margin: 0.5rem 0; white-space: pre-wrap; word-wrap: break-word; }

/* Agent detail nested details */
details.agent-detail { margin: 0.5rem 0; border: 1px solid var(--border); border-radius: 6px; }
details.agent-detail > summary { padding: 0.5rem 0.75rem; cursor: pointer; font-size: 0.85rem; font-weight: 600; background: var(--surface); border-radius: 6px; }
details.agent-detail[open] > summary { border-radius: 6px 6px 0 0; border-bottom: 1px solid var(--border); }
details.agent-detail .agent-body { padding: 0.5rem 0.75rem; font-size: 0.85rem; color: var(--muted); }

/* Agent badges */
.agent-badge { font-size: 0.7rem; font-weight: 600; padding: 0.15rem 0.6rem; border-radius: 10px; border: 1.5px solid; }
.agent-purple { background: #f5f0ff; color: #6e40c9; border-color: #d8b9ff; }
.agent-indigo { background: #eef0ff; color: #4f46e5; border-color: #c7d2fe; }
.agent-teal { background: #e6fffa; color: #0d9488; border-color: #99f6e4; }
.agent-orange { background: #fff7ed; color: #c2410c; border-color: #fed7aa; }
.agent-green { background: #ecfdf5; color: #15803d; border-color: #a7f3d0; }
.agent-red { background: #fef2f2; color: #b91c1c; border-color: #fecaca; }
.agent-blue { background: #eff6ff; color: #1d4ed8; border-color: #bfdbfe; }
.agent-default { background: var(--surface); color: var(--muted); border-color: var(--border); }

/* Vote count badge */
.vote-count { font-size: 0.7rem; font-weight: 700; padding: 0.15rem 0.6rem; border-radius: 10px; background: var(--green-bg); color: var(--green); border: 1.5px solid var(--green); }

/* Scope section headers */
.scope-section h2 { display: flex; align-items: center; gap: 0.5rem; }
.scope-section h2 .scope-count { font-size: 0.85rem; font-weight: normal; color: var(--muted); }

/* Footer */
.footer { margin-top: 2rem; padding-top: 1rem; border-top: 1px solid var(--border); font-size: 0.8rem; color: var(--muted); text-align: center; }
.footer a { color: var(--blue); text-decoration: none; }
</style>
</head>
<body>

<div class="header">
  <h1>Prism Review: PR #{{.PRNumber}}</h1>
  <div class="pr-title">{{.PRTitle}}</div>
  <div class="meta">
    <span>{{.FileCount}} files changed</span>
    <span>{{.AgentCount}} agents</span>
    <span>{{.FindingCount}} findings</span>
    {{- if .Duration}}
    <span>{{.Duration}}</span>
    {{- end}}
  </div>
</div>

{{- if gt .HealthScore.Score 0}}
<div class="gauge-container">
  <svg viewBox="0 0 200 140" width="280" height="196" role="img" aria-label="Health score: {{.HealthScore.Score}}/100 ({{.HealthScore.Grade}})">
    <!-- Background arc (gray) -->
    <path d="M 20 110 A 80 80 0 0 1 180 110" fill="none" stroke="#e1e4e8" stroke-width="14" stroke-linecap="round"/>
    <!-- Score arc (colored by score) -->
    <path d="M 20 110 A 80 80 0 0 1 180 110" fill="none" stroke="{{.GaugeColor}}" stroke-width="14" stroke-linecap="round"
          stroke-dasharray="251" stroke-dashoffset="{{.GaugeDashOffset}}"/>
    <!-- Needle -->
    <line x1="100" y1="110" x2="100" y2="40" stroke="var(--fg, #1f2328)" stroke-width="2.5" stroke-linecap="round"
          transform="rotate({{.NeedleRotation}} 100 110)"/>
    <circle cx="100" cy="110" r="5" fill="var(--fg, #1f2328)"/>
    <!-- Grade letter (large, colored, above number) -->
    <text x="100" y="80" text-anchor="middle" font-size="36" font-weight="800" fill="{{.GradeColor}}">{{.HealthScore.Grade}}</text>
    <!-- Score number (below grade) -->
    <text x="100" y="102" text-anchor="middle" font-size="18" font-weight="600" fill="var(--muted, #656d76)">{{.HealthScore.Score}} / 100</text>
  </svg>
  <div class="gauge-verdict">{{.HealthScore.Verdict}}</div>
</div>
{{- end}}

{{- if .FailedAgents}}
<div class="failed-banner">{{len .FailedAgents}} agent(s) failed: {{range $i, $a := .FailedAgents}}{{if $i}}, {{end}}{{$a}}{{end}}</div>
{{- end}}

<div class="dashboard">
  <div class="dash-card dash-card-wide">
    <div class="dash-header">
      <div>
        <div class="dash-verdict">{{.HealthScore.Verdict}}</div>
        <div class="dash-sub">{{.FindingCount}} findings from {{.AgentCount}} agents</div>
      </div>
      <div class="dash-risks">
        {{- if gt .CriticalCount 0}}<span class="dash-badge badge-critical">{{.CriticalCount}} critical</span>{{end}}
        {{- if gt .WarningCount 0}}<span class="dash-badge badge-warning">{{.WarningCount}} warning</span>{{end}}
        {{- if gt .InfoCount 0}}<span class="dash-badge badge-info">{{.InfoCount}} info</span>{{end}}
      </div>
    </div>
  </div>
</div>

{{- define "filegroup"}}
<div class="file-group">
  <div class="file-header">
    <span>{{.File}}</span>
    <div class="counts">
      {{- if gt .CriticalCount 0}}<span class="badge-critical">{{.CriticalCount}} critical</span>{{end}}
      {{- if gt .WarningCount 0}}<span class="badge-warning">{{.WarningCount}} warning</span>{{end}}
      {{- if gt .InfoCount 0}}<span class="badge-info">{{.InfoCount}} info</span>{{end}}
    </div>
  </div>
  {{range .Findings}}
  <details class="finding-card">
    <summary>
      <span class="severity {{.RiskClass}}">{{.Risk}}</span>
      <span class="cat-badge {{.CategoryClass}}">{{.Category}}</span>
      {{- if gt .VoteCount 1}}
      <span class="vote-count">{{.VoteCount}}/{{.TotalAgents}} ({{.ConsensusPercent}}%)</span>
      {{- end}}
      <span class="finding-text">{{.Summary}}</span>
      {{- if .HasLine}}
      <span class="line">:{{.Line}}</span>
      {{- end}}
    </summary>
    <div class="finding-body">
      {{- if .HasDetail}}
      <div class="overview">{{.Detail}}</div>
      {{- end}}
      {{- if .HasCodeExample}}
      <pre class="code-example">{{.CodeExample}}</pre>
      {{- end}}
      {{- if .HasMultipleAgents}}
      {{- range .AgentDetails}}
      <details class="agent-detail">
        <summary><span class="agent-badge {{.RoleClass}}">{{.Role}}</span> says {{.Risk}}</summary>
        <div class="agent-body">
          <p>{{.Detail}}</p>
          {{- if .HasCodeExample}}
          <pre class="code-example">{{.CodeExample}}</pre>
          {{- end}}
        </div>
      </details>
      {{- end}}
      {{- end}}
    </div>
  </details>
  {{end}}
</div>
{{end}}

{{- if .ChangedFindings}}
<div class="scope-section">
<h2>Issues in this PR <span class="scope-count">({{.ChangedCount}})</span></h2>
{{range .ChangedFindings}}{{template "filegroup" .}}{{end}}
</div>
{{- end}}

{{- if .ExistingFindings}}
<div class="scope-section">
<h2>Pre-existing Issues <span class="scope-count">({{.ExistingCount}})</span></h2>
{{range .ExistingFindings}}{{template "filegroup" .}}{{end}}
</div>
{{- end}}

{{- if .CodebaseFindings}}
<div class="scope-section">
<h2>Codebase Notes <span class="scope-count">({{.CodebaseCount}})</span></h2>
{{range .CodebaseFindings}}{{template "filegroup" .}}{{end}}
</div>
{{- end}}

{{if .HasUsage}}<div class="footer" style="margin-bottom: 0.5rem;">{{.InputTokensK}}k input + {{.OutputTokensK}}k output = {{.TotalTokensK}}k tokens | ${{.CostUSD}}{{if .Duration}} | {{.Duration}}{{end}}</div>{{end}}
<div class="footer">Generated by <a href="https://github.com/arinorr/prism">Prism</a></div>

</body>
</html>
`

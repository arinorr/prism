package report

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/arinorr/prism/internal/agents"
)

//go:embed markdown.tmpl
var mdTemplateFS embed.FS

// markdownData holds pre-computed values for the markdown template.
type markdownData struct {
	PRNumber        string
	PRTitle         string
	HasHealthScore  bool
	ScoreDesc       string
	ScoreGrade      string
	ScoreVerdict    string
	FileCount       int
	Roles           string
	Duration        string
	FailedAgents    []string
	Summary         string
	DedupedSections []ScopeSection
	RawSections     []RawScopeSection
	SuggestionCount int
}

var mdFuncs = template.FuncMap{
	"severityBadge": severityBadge,
	"lineStr": func(n int) string {
		if n > 0 {
			return fmt.Sprintf("%d", n)
		}
		return "-"
	},
	"votesStr": func(votes, total int) string {
		return fmt.Sprintf("%d/%d", votes, total)
	},
	"isInfo": func(r agents.Risk) bool {
		return r == severityInfo
	},
	"join": strings.Join,
}

var mdTmpl = template.Must(
	template.New("markdown.tmpl").Funcs(mdFuncs).ParseFS(mdTemplateFS, "markdown.tmpl"),
)

// Markdown generates a markdown report from the review data.
func Markdown(d *Data) string {
	md := markdownData{
		PRNumber:        d.PR.Number,
		PRTitle:         d.PR.Title,
		HasHealthScore:  d.Result.HealthScore.Score > 0 || d.Result.HealthScore.Grade != "",
		ScoreDesc:       d.Result.HealthScore.Description,
		ScoreGrade:      d.Result.HealthScore.Grade,
		ScoreVerdict:    d.Result.HealthScore.Verdict,
		FileCount:       len(d.PR.Files),
		Roles:           strings.Join(d.Roles, ", "),
		Duration:        d.Duration,
		FailedAgents:    d.Result.FailedAgents,
		Summary:         d.Result.Summary,
		SuggestionCount: len(d.Result.Suggestions),
	}

	if len(d.Result.DedupedFindings) > 0 {
		md.DedupedSections = groupDedupedByScope(d.Result.DedupedFindings)
	} else if len(d.Result.Findings) > 0 {
		md.RawSections = groupRawByScope(d.Result.Findings)
	}

	var buf bytes.Buffer
	if err := mdTmpl.Execute(&buf, md); err != nil {
		// Fallback: return the error as the report rather than panicking.
		return fmt.Sprintf("template error: %v", err)
	}
	return buf.String()
}

func severityBadge(s agents.Risk) string {
	switch s {
	case severityCritical:
		return "🔴 critical"
	case severityWarning:
		return "🟡 warning"
	default:
		return "🔵 info"
	}
}

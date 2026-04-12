package report

import (
	"encoding/json"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
)

// JSON outputs the review result as structured JSON.
func JSON(d *Data) (string, error) {
	// Ensure slices serialize as [] not null.
	findings := d.Result.Findings
	if findings == nil {
		findings = []agents.Finding{}
	}
	suggestions := toSuggestionJSON(d.Result.Suggestions)
	if suggestions == nil {
		suggestions = []suggestionJSON{}
	}
	roles := d.Roles
	if roles == nil {
		roles = []string{}
	}

	dedupedFindings := d.Result.DedupedFindings
	if dedupedFindings == nil {
		dedupedFindings = []agents.DedupedFinding{}
	}
	failedAgents := d.Result.FailedAgents
	if failedAgents == nil {
		failedAgents = []string{}
	}

	output := struct {
		PR              prSummary               `json:"pr"`
		Summary         string                  `json:"summary"`
		HealthScore     agents.HealthScore      `json:"health_score"`
		Findings        []agents.Finding        `json:"findings"`
		DedupedFindings []agents.DedupedFinding `json:"deduped_findings"`
		Suggestions     []suggestionJSON        `json:"suggestions"`
		Roles           []string                `json:"roles"`
		FailedAgents    []string                `json:"failed_agents"`
		Usage           llm.Usage               `json:"usage"`
		Duration        string                  `json:"duration,omitempty"`
		DismissedCount  int                     `json:"dismissed_count"`
		DowngradedCount int                     `json:"downgraded_count"`
		VerifierError   string                  `json:"verifier_error,omitempty"`
	}{
		PR: prSummary{
			Number: d.PR.Number,
			Title:  d.PR.Title,
			Files:  len(d.PR.Files),
		},
		Summary:         d.Result.Summary,
		HealthScore:     d.Result.HealthScore,
		Findings:        findings,
		DedupedFindings: dedupedFindings,
		Suggestions:     suggestions,
		Roles:           roles,
		FailedAgents:    failedAgents,
		Usage:           d.Usage,
		Duration:        d.Duration,
		DismissedCount:  d.DismissedCount,
		DowngradedCount: d.DowngradedCount,
		VerifierError:   d.VerifierError,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type prSummary struct {
	Number string `json:"number"`
	Title  string `json:"title"`
	Files  int    `json:"files"`
}

type suggestionJSON struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Body string `json:"body"`
	Role string `json:"role"`
}

func toSuggestionJSON(suggestions []gh.Suggestion) []suggestionJSON {
	out := make([]suggestionJSON, len(suggestions))
	for i, s := range suggestions {
		out[i] = suggestionJSON{File: s.File, Line: s.Line, Body: s.Body, Role: s.Role}
	}
	return out
}

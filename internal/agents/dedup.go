package agents

import (
	"sort"
	"strings"
)

const (
	// lineThreshold is the maximum line distance for two findings to be
	// considered duplicates of the same issue.
	lineThreshold = 5

	// jaccardThreshold is the minimum Jaccard similarity between two
	// finding summaries for them to be considered duplicates.
	jaccardThreshold = 0.4
)

// AgentDetail captures one agent's individual perspective on a finding.
type AgentDetail struct {
	Role        string `json:"role"`
	Risk        string `json:"risk"` // this agent's individual severity opinion
	Detail      string `json:"detail"`
	CodeExample string `json:"code_example,omitempty"`
}

// DedupedFinding wraps a Finding with vote metadata from deduplication.
type DedupedFinding struct {
	Finding
	VoteCount         int           `json:"vote_count"`
	TotalAgents       int           `json:"total_agents"`
	Voters            []string      `json:"voters"`
	AgentDetails      []AgentDetail `json:"agent_details"`
	Confidence        float64       `json:"confidence"`         // VoteCount/TotalAgents (0.0-1.0)
	CompositeSeverity float64       `json:"composite_severity"` // weighted average (1.0-3.0)
}

// Deduplicate merges findings that refer to the same issue.
// Two findings match if they have the same file, lines within lineThreshold,
// and Jaccard similarity of summaries above jaccardThreshold.
// Results are sorted by vote count (desc), severity, file, then line.
func Deduplicate(findings []Finding, totalAgents int) []DedupedFinding {
	if len(findings) == 0 {
		return []DedupedFinding{}
	}

	var groups []DedupedFinding

	for fi := range findings {
		f := &findings[fi]
		idx := -1
		for i := range groups {
			if matchesGroup(&groups[i], f) {
				idx = i
				break
			}
		}
		ad := AgentDetail{Role: f.Role, Risk: f.Risk, Detail: f.Detail, CodeExample: f.CodeExample}
		if idx < 0 {
			groups = append(groups, DedupedFinding{
				Finding:      *f,
				VoteCount:    1,
				TotalAgents:  totalAgents,
				Voters:       []string{f.Role},
				AgentDetails: []AgentDetail{ad},
			})
			continue
		}
		groups[idx].VoteCount++
		groups[idx].Voters = append(groups[idx].Voters, f.Role)
		groups[idx].AgentDetails = append(groups[idx].AgentDetails, ad)
		if len(f.Detail) > len(groups[idx].Detail) {
			groups[idx].Detail = f.Detail
		}
		if len(f.CodeExample) > len(groups[idx].CodeExample) {
			groups[idx].CodeExample = f.CodeExample
		}
		// Severity is no longer "highest wins" — computed below as composite.
	}

	// Compute Confidence and CompositeSeverity for each group.
	for i := range groups {
		g := &groups[i]
		g.Confidence = float64(g.VoteCount) / float64(g.TotalAgents)
		g.CompositeSeverity = computeCompositeSeverity(g.AgentDetails, g.Category)
		g.Risk = compositeSeverityToLabel(g.CompositeSeverity)
	}

	// Sort: vote count desc, severity asc, file asc, line asc.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].VoteCount != groups[j].VoteCount {
			return groups[i].VoteCount > groups[j].VoteCount
		}
		si, sj := SeverityOrder(groups[i].Risk), SeverityOrder(groups[j].Risk)
		if si != sj {
			return si < sj
		}
		if groups[i].File != groups[j].File {
			return groups[i].File < groups[j].File
		}
		return groups[i].Line < groups[j].Line
	})

	return groups
}

// Severity level constants. Exported for use by the report package.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// SeverityOrder returns a sort key for severity (0=critical, 1=warning, 2=info).
func SeverityOrder(s string) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	default:
		return 2
	}
}

// Composite severity constants.
const (
	criticalGravityMultiplier  = 2.0 // critical votes are harder to override
	domainAuthorityMultiplier  = 1.5 // domain experts carry more weight
	compositeCriticalThreshold = 2.5
	compositeWarningThreshold  = 1.7
)

// SeverityNumeric converts a severity label to a numeric value.
func SeverityNumeric(s string) float64 {
	switch s {
	case SeverityCritical:
		return 3.0
	case SeverityWarning:
		return 2.0
	default:
		return 1.0
	}
}

// computeCompositeSeverity calculates a weighted average severity from all
// agent votes. Critical votes get extra weight (gravity), and domain-authority
// votes carry more weight when the finding category matches their specialty.
func computeCompositeSeverity(details []AgentDetail, findingCategory string) float64 {
	if len(details) == 0 {
		return 1.0
	}
	var weightedSum, totalWeight float64
	for _, d := range details {
		w := 1.0
		if d.Risk == SeverityCritical {
			w *= criticalGravityMultiplier
		}
		if IsDomainAuthority(d.Role, findingCategory) {
			w *= domainAuthorityMultiplier
		}
		weightedSum += SeverityNumeric(d.Risk) * w
		totalWeight += w
	}
	return weightedSum / totalWeight
}

// compositeSeverityToLabel maps a numeric composite back to a severity label.
func compositeSeverityToLabel(cs float64) string {
	switch {
	case cs >= compositeCriticalThreshold:
		return SeverityCritical
	case cs >= compositeWarningThreshold:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// DisagreementSpread returns the numeric spread between the highest and lowest
// severity opinions on a deduped finding. A spread of 2 means critical vs info.
func DisagreementSpread(details []AgentDetail) int {
	if len(details) < 2 {
		return 0
	}
	minSev, maxSev := 3, 1
	for _, d := range details {
		n := int(SeverityNumeric(d.Risk))
		if n < minSev {
			minSev = n
		}
		if n > maxSev {
			maxSev = n
		}
	}
	return maxSev - minSev
}

func matchesGroup(group *DedupedFinding, f *Finding) bool {
	if group.File != f.File {
		return false
	}
	if abs(group.Line-f.Line) > lineThreshold {
		return false
	}
	return jaccardSimilarity(group.Summary, f.Summary) >= jaccardThreshold
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// jaccardSimilarity computes the Jaccard index of two strings tokenized on whitespace.
func jaccardSimilarity(a, b string) float64 {
	tokensA := tokenize(a)
	tokensB := tokenize(b)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}

	union := make(map[string]bool)
	for t := range tokensA {
		union[t] = true
	}
	for t := range tokensB {
		union[t] = true
	}

	if len(union) == 0 {
		return 0
	}

	var intersection int
	for t := range tokensA {
		if tokensB[t] {
			intersection++
		}
	}

	return float64(intersection) / float64(len(union))
}

func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	for _, word := range strings.Fields(strings.ToLower(s)) {
		// Strip common punctuation.
		word = strings.Trim(word, ".,;:!?\"'`()[]{}—-")
		if word != "" {
			tokens[word] = true
		}
	}
	return tokens
}

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
	Detail      string `json:"detail"`
	CodeExample string `json:"code_example,omitempty"`
}

// DedupedFinding wraps a Finding with vote metadata from deduplication.
type DedupedFinding struct {
	Finding
	VoteCount    int           `json:"vote_count"`
	TotalAgents  int           `json:"total_agents"`
	Voters       []string      `json:"voters"`
	AgentDetails []AgentDetail `json:"agent_details"`
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
		ad := AgentDetail{Role: f.Role, Detail: f.Detail, CodeExample: f.CodeExample}
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
		if SeverityOrder(f.Risk) < SeverityOrder(groups[idx].Risk) {
			groups[idx].Risk = f.Risk
		}
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

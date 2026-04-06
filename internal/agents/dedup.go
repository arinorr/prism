package agents

import (
	"sort"
	"strings"
)

const (
	// lineThreshold is the maximum line distance for two findings to be
	// considered duplicates using the standard text similarity threshold.
	lineThreshold = 5

	// wideLineThreshold is used when structural signals are strong (same
	// file + same category). Findings within this range need less text
	// similarity to be considered duplicates.
	wideLineThreshold = 20

	// jaccardThreshold is the baseline minimum Jaccard similarity for
	// findings that don't share strong structural signals.
	jaccardThreshold = 0.4

	// sameCategoryLineLimit caps the line distance for tier 2 matching.
	// Prevents merging genuinely distinct findings at opposite ends of
	// large files that happen to share a category and modest word overlap.
	sameCategoryLineLimit = 100

	// jaccardThresholdSameCategory is the threshold when findings share
	// the same file and category within sameCategoryLineLimit lines —
	// structural agreement compensates for wording differences.
	jaccardThresholdSameCategory = 0.15

	// jaccardThresholdNearby is the threshold when findings share the
	// same file, same category, AND are within wideLineThreshold lines.
	// Very lenient, but still requires minimal text overlap to avoid
	// merging genuinely distinct findings at nearby lines.
	jaccardThresholdNearby = 0.065
)

// Composite severity constants.
const (
	criticalGravityMultiplier  = 2.0 // critical votes are harder to override
	domainAuthorityMultiplier  = 1.5 // domain experts carry more weight
	compositeCriticalThreshold = 2.5 // composite >= this → critical
	compositeWarningThreshold  = 1.7 // composite >= this → warning
)

// AgentDetail captures one agent's individual perspective on a finding.
type AgentDetail struct {
	Role        string `json:"role"`
	Risk        Risk   `json:"risk"` // this agent's individual severity opinion
	Detail      string `json:"detail"`
	CodeExample string `json:"code_example,omitempty"`
}

// DedupedFinding wraps a Finding with vote metadata from deduplication.
type DedupedFinding struct {
	Finding
	VoteCount    int               `json:"vote_count"`
	TotalAgents  int               `json:"total_agents"`
	Voters       []string          `json:"voters"`
	AgentDetails []AgentDetail     `json:"agent_details"`
	voterTokens  []map[string]bool // cached tokenized summaries to avoid re-tokenization
}

// Consensus returns the fraction of agents that flagged this issue (0.0-1.0).
// Distinct from Finding.Confidence which is the LLM's self-assessed confidence.
func (df *DedupedFinding) Consensus() float64 {
	if df.TotalAgents == 0 {
		return 0
	}
	return float64(df.VoteCount) / float64(df.TotalAgents)
}

// CompositeSeverity returns a weighted average severity (1.0-3.0) computed from
// all agents' individual opinions. Critical votes carry 2x weight, and domain
// experts carry 1.5x weight for findings in their specialty.
func (df *DedupedFinding) CompositeSeverity() float64 {
	return computeCompositeSeverity(df.AgentDetails, df.Category)
}

// SeverityNumeric maps a risk label to a numeric value for weighted averaging.
func SeverityNumeric(r Risk) float64 {
	switch r {
	case RiskCritical:
		return 3.0
	case RiskWarning:
		return 2.0
	default:
		return 1.0
	}
}

// computeCompositeSeverity calculates a weighted average severity from agent votes.
// Critical votes get extra weight (gravity), and domain-authority votes carry
// more weight when the finding category matches their specialty.
func computeCompositeSeverity(details []AgentDetail, findingCategory string) float64 {
	if len(details) == 0 {
		return 1.0
	}
	var weightedSum, totalWeight float64
	for _, d := range details {
		w := 1.0
		if d.Risk == RiskCritical {
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

// compositeSeverityToRisk maps a numeric composite back to a risk label.
func compositeSeverityToRisk(cs float64) Risk {
	switch {
	case cs >= compositeCriticalThreshold:
		return RiskCritical
	case cs >= compositeWarningThreshold:
		return RiskWarning
	default:
		return RiskInfo
	}
}

// DisagreementSpread returns the spread between the highest and lowest severity
// opinions. A spread of 2 means critical vs info — triggers debate.
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

// Deduplicate merges findings that refer to the same issue using hybrid
// scoring: structural signals (same file, same category, line proximity)
// lower the text similarity threshold. This catches semantically identical
// findings even when agents use different wording.
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
				voterTokens:  []map[string]bool{tokenize(f.Summary)},
			})
			continue
		}
		groups[idx].VoteCount++
		groups[idx].Voters = append(groups[idx].Voters, f.Role)
		groups[idx].AgentDetails = append(groups[idx].AgentDetails, ad)
		groups[idx].voterTokens = append(groups[idx].voterTokens, tokenize(f.Summary))
		if len(f.Detail) > len(groups[idx].Detail) {
			groups[idx].Detail = f.Detail
		}
		if len(f.CodeExample) > len(groups[idx].CodeExample) {
			groups[idx].CodeExample = f.CodeExample
		}
		// Risk is computed below as composite — not "highest wins."
	}

	// Compute composite severity and derive Risk label for each group.
	// Invariant: after this loop, DedupedFinding fields are frozen.
	// Data flow: dispatch → debate (mutates raw feedbacks) → dedup (freeze) → score.
	for i := range groups {
		groups[i].Risk = compositeSeverityToRisk(groups[i].CompositeSeverity())
	}

	// Sort: vote count desc, severity asc, file asc, line asc.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].VoteCount != groups[j].VoteCount {
			return groups[i].VoteCount > groups[j].VoteCount
		}
		si, sj := groups[i].Risk.Order(), groups[j].Risk.Order()
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

// bestSimilarity returns the highest Jaccard similarity between the
// candidate's pre-tokenized summary and any cached token set in the group.
// If voterTokens is empty (zero-value struct), it derives tokens from
// the embedded Finding's Summary so the zero value works without
// initialization ceremony.
func bestSimilarity(group *DedupedFinding, candidateTokens map[string]bool) float64 {
	tokens := group.voterTokens
	if len(tokens) == 0 {
		tokens = []map[string]bool{tokenize(group.Summary)}
	}
	var best float64
	for _, gt := range tokens {
		if sim := jaccardFromTokens(gt, candidateTokens); sim > best {
			best = sim
		}
	}
	return best
}

// jaccardFromTokens computes Jaccard similarity from pre-tokenized sets
// without allocating a union map: |union| = |a| + |b| - |intersection|.
func jaccardFromTokens(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	var intersection int
	for t := range a {
		if b[t] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// matchesGroup uses hybrid scoring: structural signals (same file, same
// category, line proximity) lower the text similarity threshold needed to
// consider two findings as duplicates. This catches semantically identical
// findings that use different wording.
//
// Tiers:
//  1. Same file + same category + within 20 lines  → threshold 0.065
//  2. Same file + same category + within 100 lines → threshold 0.15
//  3. Same file + within 5 lines                   → threshold 0.40
//  4. Otherwise                                    → no match
//
// Empty categories do not qualify for tiers 1/2 to avoid false merges
// when agents omit the category field.
func matchesGroup(group *DedupedFinding, f *Finding) bool {
	if group.File != f.File {
		return false
	}

	lineDist := abs(group.Line - f.Line)
	sameCategory := group.Category != "" && group.Category == f.Category

	// Tokenize candidate once, then compare against cached group tokens.
	candidateTokens := tokenize(f.Summary)
	similarity := bestSimilarity(group, candidateTokens)

	// Tier 1: strong structural match — same file, same category, nearby lines.
	if sameCategory && lineDist <= wideLineThreshold {
		return similarity >= jaccardThresholdNearby
	}

	// Tier 2: same file and category, within sameCategoryLineLimit.
	if sameCategory && lineDist <= sameCategoryLineLimit {
		return similarity >= jaccardThresholdSameCategory
	}

	// Tier 3: same file, close lines, but different categories.
	if lineDist <= lineThreshold {
		return similarity >= jaccardThreshold
	}

	return false
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
		if word == "" {
			continue
		}
		tokens[word] = true
		// Add a prefix stem (first 6 chars) so "duplicate" and
		// "duplication" share a token. This is a lightweight alternative
		// to full stemming that helps with the most common suffix
		// variations without external dependencies.
		if len(word) > 6 {
			tokens[word[:6]] = true
		}
	}
	return tokens
}

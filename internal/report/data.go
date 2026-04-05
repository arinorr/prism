package report

import (
	"sort"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/gh"
	"github.com/arinorr/prism/internal/llm"
)

// Use severity constants from agents package to avoid duplication.
const (
	severityCritical = agents.RiskCritical
	severityWarning  = agents.RiskWarning
	severityInfo     = agents.RiskInfo
)

// Data holds everything needed to generate a report.
type Data struct {
	PR       *gh.PR
	Result   *agents.ReviewResult
	Roles    []string
	Duration string
	Usage    llm.Usage
}

const generalFile = "(general)"

type scopeSection struct {
	title  string
	groups []dedupedFileGroup
}

type rawScopeSection struct {
	title  string
	groups []fileGroup
}

// scopeTitles maps scope values to their markdown section titles.
var scopeTitles = map[string]string{
	agents.ScopeChanged:  "Issues in this PR",
	agents.ScopeExisting: "Pre-existing Issues",
	agents.ScopeCodebase: "Codebase Notes",
}

type fileGroup struct {
	file     string
	findings []agents.Finding
}

type dedupedFileGroup struct {
	file     string
	findings []agents.DedupedFinding
}

func groupByFile(findings []agents.Finding) []fileGroup {
	byFile := make(map[string][]agents.Finding)
	for i := range findings {
		file := findings[i].File
		if file == "" {
			file = generalFile
		}
		byFile[file] = append(byFile[file], findings[i])
	}

	groups := make([]fileGroup, 0, len(byFile))
	for file, fs := range byFile {
		sort.Slice(fs, func(i, j int) bool {
			si, sj := severityOrder(fs[i].Risk), severityOrder(fs[j].Risk)
			if si != sj {
				return si < sj
			}
			return fs[i].Line < fs[j].Line
		})
		groups = append(groups, fileGroup{file: file, findings: fs})
	}

	// Sort by severity: files with critical findings first, then warning, then alphabetically.
	sortRawFileGroups(groups)

	return groups
}

func groupDedupedByFile(findings []agents.DedupedFinding) []dedupedFileGroup {
	byFile := make(map[string][]agents.DedupedFinding)
	for i := range findings {
		file := findings[i].File
		if file == "" {
			file = generalFile
		}
		byFile[file] = append(byFile[file], findings[i])
	}

	groups := make([]dedupedFileGroup, 0, len(byFile))
	for file, fs := range byFile {
		sort.Slice(fs, func(i, j int) bool {
			if fs[i].VoteCount != fs[j].VoteCount {
				return fs[i].VoteCount > fs[j].VoteCount
			}
			si, sj := severityOrder(fs[i].Risk), severityOrder(fs[j].Risk)
			if si != sj {
				return si < sj
			}
			return fs[i].Line < fs[j].Line
		})
		groups = append(groups, dedupedFileGroup{file: file, findings: fs})
	}

	// Sort by severity: files with critical findings first, then warning, then alphabetically.
	sortDedupedFileGroups(groups)

	return groups
}

func groupDedupedByScope(findings []agents.DedupedFinding) []scopeSection {
	scoped := map[string][]agents.DedupedFinding{}
	for i := range findings {
		scope := findings[i].Scope
		if scope == "" {
			scope = agents.ScopeChanged
		}
		scoped[scope] = append(scoped[scope], findings[i])
	}

	var sections []scopeSection
	for _, s := range []string{agents.ScopeChanged, agents.ScopeExisting, agents.ScopeCodebase} {
		if fs, ok := scoped[s]; ok && len(fs) > 0 {
			sections = append(sections, scopeSection{
				title:  scopeTitles[s],
				groups: groupDedupedByFile(fs),
			})
		}
	}
	return sections
}

func groupRawByScope(findings []agents.Finding) []rawScopeSection {
	scoped := map[string][]agents.Finding{}
	for i := range findings {
		scope := findings[i].Scope
		if scope == "" {
			scope = agents.ScopeChanged
		}
		scoped[scope] = append(scoped[scope], findings[i])
	}

	var sections []rawScopeSection
	for _, s := range []string{agents.ScopeChanged, agents.ScopeExisting, agents.ScopeCodebase} {
		if fs, ok := scoped[s]; ok && len(fs) > 0 {
			sections = append(sections, rawScopeSection{
				title:  scopeTitles[s],
				groups: groupByFile(fs),
			})
		}
	}
	return sections
}

// splitByScope distributes file groups into scope buckets. Each finding within
// a file group may have a different scope, so we re-bucket at the finding level.
func splitByScope(groups []htmlFileGroup) (changed, existing, codebase []htmlFileGroup) {
	// Collect findings per (scope, file).
	type key struct{ scope, file string }
	buckets := make(map[key][]htmlFinding)
	for gi := range groups {
		for fi := range groups[gi].Findings {
			f := &groups[gi].Findings[fi]
			scope := f.Scope
			if scope == "" {
				scope = agents.ScopeChanged
			}
			k := key{scope, groups[gi].File}
			buckets[k] = append(buckets[k], *f)
		}
	}

	buildGroups := func(scope string) []htmlFileGroup {
		var out []htmlFileGroup
		for k, findings := range buckets {
			if k.scope != scope {
				continue
			}
			var fc, fw, fi int
			for i := range findings {
				switch findings[i].Risk {
				case severityCritical:
					fc++
				case severityWarning:
					fw++
				default:
					fi++
				}
			}
			out = append(out, htmlFileGroup{
				File:          k.file,
				CriticalCount: fc,
				WarningCount:  fw,
				InfoCount:     fi,
				Findings:      findings,
			})
		}
		sort.Slice(out, func(i, j int) bool { return htmlFileGroupLess(out[i], out[j]) })
		return out
	}

	changed = buildGroups(agents.ScopeChanged)
	existing = buildGroups(agents.ScopeExisting)
	codebase = buildGroups(agents.ScopeCodebase)
	return
}

// htmlFileGroupLess defines the canonical sort contract for file ordering:
// critical count desc -> warning count desc -> filename asc.
// Used directly by splitByScope; sortRawFileGroups and sortDedupedFileGroups
// implement the same contract independently via severityKey.
func htmlFileGroupLess(a, b htmlFileGroup) bool {
	if a.CriticalCount != b.CriticalCount {
		return a.CriticalCount > b.CriticalCount
	}
	if a.WarningCount != b.WarningCount {
		return a.WarningCount > b.WarningCount
	}
	return a.File < b.File
}

// severityKey holds precomputed severity counts for sorting raw/deduped
// file groups before they're converted to htmlFileGroups.
type severityKey struct {
	critical, warning int
}

func sortRawFileGroups(groups []fileGroup) {
	if len(groups) <= 1 {
		return
	}
	keys := make([]severityKey, len(groups))
	for i := range groups {
		for j := range groups[i].findings {
			switch groups[i].findings[j].Risk {
			case severityCritical:
				keys[i].critical++
			case severityWarning:
				keys[i].warning++
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if keys[i].critical != keys[j].critical {
			return keys[i].critical > keys[j].critical
		}
		if keys[i].warning != keys[j].warning {
			return keys[i].warning > keys[j].warning
		}
		return groups[i].file < groups[j].file
	})
}

func sortDedupedFileGroups(groups []dedupedFileGroup) {
	if len(groups) <= 1 {
		return
	}
	keys := make([]severityKey, len(groups))
	for i := range groups {
		for j := range groups[i].findings {
			switch groups[i].findings[j].Risk {
			case severityCritical:
				keys[i].critical++
			case severityWarning:
				keys[i].warning++
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if keys[i].critical != keys[j].critical {
			return keys[i].critical > keys[j].critical
		}
		if keys[i].warning != keys[j].warning {
			return keys[i].warning > keys[j].warning
		}
		return groups[i].file < groups[j].file
	})
}

// severityOrder delegates to Risk.Order().
func severityOrder(r agents.Risk) int { return r.Order() }

package agents

import (
	"fmt"
	"testing"
)

// BenchmarkDeduplicate_ManyIdentical benchmarks dedup with many similar
// findings (high dedup ratio — typical for SQL injection found by all agents).
func BenchmarkDeduplicate_ManyIdentical(b *testing.B) {
	findings := make([]Finding, 7)
	for i := range findings {
		findings[i] = Finding{
			File:     "auth/login.go",
			Line:     42,
			Risk:     RiskCritical,
			Category: "security",
			Scope:    ScopeChanged,
			Summary:  fmt.Sprintf("SQL injection vulnerability in login query (agent %d)", i),
			Detail:   "The login function uses string concatenation for SQL.",
			Role:     fmt.Sprintf("agent-%d", i),
		}
	}
	b.ResetTimer()
	for range b.N {
		Deduplicate(findings, 7)
	}
}

// BenchmarkDeduplicate_AllUnique benchmarks dedup with findings that don't
// overlap (worst case — nothing to merge, maximum comparison work).
func BenchmarkDeduplicate_AllUnique(b *testing.B) {
	findings := make([]Finding, 50)
	for i := range findings {
		findings[i] = Finding{
			File:     fmt.Sprintf("file%d.go", i),
			Line:     i * 10,
			Risk:     RiskWarning,
			Category: "design",
			Scope:    ScopeChanged,
			Summary:  fmt.Sprintf("Unique issue number %d in a completely different file", i),
			Detail:   fmt.Sprintf("Detail for finding %d that shares no words with others", i),
			Role:     fmt.Sprintf("agent-%d", i%7),
		}
	}
	b.ResetTimer()
	for range b.N {
		Deduplicate(findings, 7)
	}
}

// BenchmarkDeduplicate_RealisticMix benchmarks dedup with a mix of
// overlapping and unique findings (typical real-world scenario).
func BenchmarkDeduplicate_RealisticMix(b *testing.B) {
	var findings []Finding
	// 4 agents find the same SQL injection.
	for i := 0; i < 4; i++ {
		findings = append(findings, Finding{
			File:     "auth/login.go",
			Line:     42,
			Risk:     RiskCritical,
			Category: "security",
			Scope:    ScopeChanged,
			Summary:  fmt.Sprintf("SQL injection in login query (variant %d)", i),
			Detail:   "Uses string concatenation.",
			Role:     fmt.Sprintf("agent-%d", i),
		})
	}
	// 2 agents find the same design issue.
	for i := 0; i < 2; i++ {
		findings = append(findings, Finding{
			File:     "auth/session.go",
			Line:     15,
			Risk:     RiskWarning,
			Category: "design",
			Scope:    ScopeChanged,
			Summary:  fmt.Sprintf("Session timeout too long (variant %d)", i),
			Detail:   "24h is excessive.",
			Role:     fmt.Sprintf("agent-%d", i+4),
		})
	}
	// 5 unique findings from individual agents.
	for i := 0; i < 5; i++ {
		findings = append(findings, Finding{
			File:     fmt.Sprintf("pkg/module%d.go", i),
			Line:     i*20 + 5,
			Risk:     RiskInfo,
			Category: "style",
			Scope:    ScopeExisting,
			Summary:  fmt.Sprintf("Style suggestion %d for different module", i),
			Detail:   fmt.Sprintf("Unique detail %d", i),
			Role:     fmt.Sprintf("agent-%d", i%7),
		})
	}
	b.ResetTimer()
	for range b.N {
		Deduplicate(findings, 7)
	}
}

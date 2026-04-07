package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/arinorr/prism/internal/llm"
	"github.com/arinorr/prism/internal/resolve"
)

// VerificationStatus indicates the result of finding verification.
type VerificationStatus string

const (
	StatusConfirmed  VerificationStatus = "confirmed"
	StatusDismissed  VerificationStatus = "dismissed"
	StatusDowngraded VerificationStatus = "downgraded"
	StatusUnverified VerificationStatus = "unverified"
)

// maxFindingsPerCall caps how many findings are sent in a single LLM call
// to prevent context blowup for files with many findings.
const maxFindingsPerCall = 5

// Verifier verifies deduped findings against full codebase context using
// a two-tier LLM strategy: Haiku for cheap factual checks, then Opus for
// judgment calls on uncertain findings.
type Verifier struct {
	llm      llm.LLM
	resolver *resolve.Resolver
	opts     *Options
	budget   float64 // max USD for verification; 0 = no limit
	spent    float64 // cumulative USD spent
}

// NewVerifier creates a Verifier. The budget defaults to 0 (no limit);
// the caller should set it based on estimated agent costs.
func NewVerifier(backend llm.LLM, resolver *resolve.Resolver, opts *Options) *Verifier {
	budget := opts.VerifierBudgetUSD
	return &Verifier{
		llm:      backend,
		resolver: resolver,
		opts:     opts,
		budget:   budget,
	}
}

// verdict is the result of a single finding's verification.
type verdict struct {
	Status       VerificationStatus
	Reason       string
	AdjustedRisk Risk // only meaningful for StatusDowngraded
}

// indexedFinding pairs a finding with its position and resolved context.
type indexedFinding struct {
	idx     int
	finding DedupedFinding
	ctx     *resolve.ResolvedContext
}

// Verify runs two-tier verification on the given findings:
//  1. Resolve codebase context for each finding via the symbol index.
//  2. Haiku tier: factual evidence checks (YES/NO/UNSURE).
//  3. Opus tier: judgment calls on UNSURE findings only.
//
// Dismissed findings are filtered out. Downgraded findings have their
// risk adjusted. Findings that exceed the budget pass through as unverified.
func (v *Verifier) Verify(ctx context.Context, findings []DedupedFinding) ([]DedupedFinding, llm.Usage, error) {
	if len(findings) == 0 {
		return findings, llm.Usage{}, nil
	}

	var totalUsage llm.Usage

	// Pass 1: preload finding files.
	fileSet := make(map[string]bool)
	for _, f := range findings {
		if f.File != "" {
			fileSet[f.File] = true
		}
	}
	findingFiles := make([]string, 0, len(fileSet))
	for f := range fileSet {
		findingFiles = append(findingFiles, f)
	}
	v.resolver.PreloadFiles(findingFiles)

	// Pass 2: collect reference files and preload those too.
	refFiles := v.resolver.CollectReferenceFiles(findingFiles)
	v.resolver.PreloadFiles(refFiles)

	// Resolve context for each finding.
	contexts := make([]*resolve.ResolvedContext, len(findings))
	for i, f := range findings {
		if f.File == "" {
			continue
		}
		rc, err := v.resolver.Resolve(f.File, f.Line)
		if err != nil {
			continue // no context available; will pass through
		}
		contexts[i] = rc
	}

	// Group findings by file for batching.
	byFile := make(map[string][]indexedFinding)
	for i, f := range findings {
		byFile[f.File] = append(byFile[f.File], indexedFinding{i, f, contexts[i]})
	}

	// Haiku tier: factual checks.
	verdicts := make([]verdict, len(findings))
	for i := range verdicts {
		verdicts[i] = verdict{Status: StatusConfirmed} // default: pass through
	}

	for _, group := range byFile {
		// Batch into chunks of maxFindingsPerCall.
		for start := 0; start < len(group); start += maxFindingsPerCall {
			if v.budgetExceeded() {
				for j := start; j < len(group); j++ {
					verdicts[group[j].idx] = verdict{Status: StatusUnverified, Reason: "verifier budget exceeded"}
				}
				break
			}

			end := start + maxFindingsPerCall
			if end > len(group) {
				end = len(group)
			}
			batch := group[start:end]

			batchVerdicts, usage, err := v.runHaikuBatch(ctx, batch)
			totalUsage = totalUsage.Add(usage)
			v.spent += usage.CostUSD

			if err != nil {
				// Haiku failed: all findings in batch are UNSURE (escalate to Opus).
				continue
			}

			for j, bv := range batchVerdicts {
				verdicts[batch[j].idx] = bv
			}
		}
	}

	// Opus tier: judgment on uncertain findings.
	for _, group := range byFile {
		for _, item := range group {
			vd := verdicts[item.idx]
			if vd.Status != StatusConfirmed || vd.Reason != "" {
				continue // already decided by Haiku (dismissed, confirmed with reason, or budget exceeded)
			}

			// Check if Haiku returned a definitive YES — those are confirmed, skip Opus.
			// Only escalate findings that are genuinely unsure (default verdict, no reason set).
			// We use an empty reason as the signal that this was a default/unsure verdict.

			if v.budgetExceeded() {
				verdicts[item.idx] = verdict{Status: StatusUnverified, Reason: "verifier budget exceeded"}
				continue
			}

			opusVerdict, usage, err := v.runOpusSingle(ctx, item.finding, item.ctx)
			totalUsage = totalUsage.Add(usage)
			v.spent += usage.CostUSD

			if err != nil {
				// Opus failed: finding passes through as confirmed (safe fallback).
				verdicts[item.idx] = verdict{Status: StatusConfirmed, Reason: "verification inconclusive"}
				continue
			}

			verdicts[item.idx] = opusVerdict
		}
	}

	// Apply verdicts to findings.
	var result []DedupedFinding
	for i, f := range findings {
		vd := verdicts[i]
		f.VerificationStatus = vd.Status
		f.VerificationReason = vd.Reason

		switch vd.Status {
		case StatusDismissed:
			continue // filter out
		case StatusDowngraded:
			if vd.AdjustedRisk.Valid() {
				f.Risk = vd.AdjustedRisk
			}
		}

		result = append(result, f)
	}

	return result, totalUsage, nil
}

func (v *Verifier) budgetExceeded() bool {
	return v.budget > 0 && v.spent >= v.budget
}

// --- Haiku tier ---

const haikuSystemPrompt = `You are a code analysis fact-checker. You will be given code review findings and the relevant source code context.

For each numbered finding, determine if the issue ACTUALLY EXISTS in the provided code. Consider:
- Does the enclosing function/class actually exhibit the described problem?
- Is the issue handled elsewhere in the referenced code (e.g., validation upstream, nil checks, type guards)?
- Could the reviewer have missed context that resolves the concern?

For each finding, respond with exactly one line in this format:
N. YES|NO|UNSURE — brief explanation

Where N is the finding number. Be precise and concise.`

func (v *Verifier) runHaikuBatch(ctx context.Context, batch []indexedFinding) ([]verdict, llm.Usage, error) {
	prompt := buildHaikuPrompt(batch)

	resp, usage, err := v.llm.Complete(ctx, llm.Request{
		SystemPrompt: haikuSystemPrompt,
		UserPrompt:   prompt,
		Model:        ModelTierFast,
	})
	if err != nil {
		return nil, usage, fmt.Errorf("haiku verification: %w", err)
	}

	verdicts := parseHaikuResponse(resp, len(batch))
	return verdicts, usage, nil
}

func buildHaikuPrompt(batch []indexedFinding) string {
	var b strings.Builder

	for i, item := range batch {
		fmt.Fprintf(&b, "## Finding %d\n", i+1)
		fmt.Fprintf(&b, "File: %s, Line: %d\n", item.finding.File, item.finding.Line)
		fmt.Fprintf(&b, "Risk: %s, Category: %s\n", item.finding.Risk, item.finding.Category)
		fmt.Fprintf(&b, "Summary: %s\n", item.finding.Summary)
		fmt.Fprintf(&b, "Detail: %s\n\n", item.finding.Detail)

		if item.ctx != nil {
			if item.ctx.EnclosingScope != nil {
				fmt.Fprintf(&b, "### Enclosing scope: %s (%s, lines %d-%d)\n```\n%s\n```\n\n",
					item.ctx.EnclosingScope.Symbol.Name,
					item.ctx.EnclosingScope.Symbol.Kind,
					item.ctx.EnclosingScope.Symbol.StartLine,
					item.ctx.EnclosingScope.Symbol.EndLine,
					item.ctx.EnclosingScope.Text)
			}

			for _, ref := range item.ctx.References {
				fmt.Fprintf(&b, "### Referenced: %s (%s in %s, lines %d-%d)\n```\n%s\n```\n\n",
					ref.Symbol.Name,
					ref.Symbol.Kind,
					ref.Symbol.File,
					ref.Symbol.StartLine,
					ref.Symbol.EndLine,
					ref.Text)
			}
		}
	}

	return b.String()
}

func parseHaikuResponse(response string, count int) []verdict {
	verdicts := make([]verdict, count)
	// Default: unsure (escalate to Opus).
	for i := range verdicts {
		verdicts[i] = verdict{Status: StatusConfirmed} // will be overwritten
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse "N. YES|NO|UNSURE — reason"
		var num int
		var rest string
		if _, err := fmt.Sscanf(line, "%d.", &num); err != nil {
			continue
		}
		// Extract after "N. "
		dotIdx := strings.Index(line, ".")
		if dotIdx < 0 || dotIdx+1 >= len(line) {
			continue
		}
		rest = strings.TrimSpace(line[dotIdx+1:])

		if num < 1 || num > count {
			continue
		}

		upper := strings.ToUpper(rest)
		reason := ""
		// Extract reason after separator (— or -)
		for _, sep := range []string{" — ", " - ", ": "} {
			if idx := strings.Index(rest, sep); idx >= 0 {
				reason = strings.TrimSpace(rest[idx+len(sep):])
				break
			}
		}

		idx := num - 1
		switch {
		case strings.HasPrefix(upper, "YES"):
			verdicts[idx] = verdict{Status: StatusConfirmed, Reason: reason}
		case strings.HasPrefix(upper, "NO"):
			verdicts[idx] = verdict{Status: StatusDismissed, Reason: reason}
		case strings.HasPrefix(upper, "UNSURE"):
			// Leave as default (will escalate to Opus).
			verdicts[idx] = verdict{Status: StatusConfirmed, Reason: ""} // empty reason = unsure = escalate
		}
	}

	return verdicts
}

// --- Opus tier ---

const opusSystemPrompt = `You are a senior code reviewer verifying findings from an automated review. You have full source code context that the original reviewers did not have.

For each finding, determine:
- "confirmed": The issue is real and correctly categorized.
- "dismissed": The issue is a false positive — the code is actually correct (explain why).
- "downgraded": The issue exists but is less severe than reported (provide the adjusted risk level).

Respond with a JSON object:
{"verdict": "confirmed|dismissed|downgraded", "reason": "...", "adjusted_risk": "critical|warning|info"}

The adjusted_risk field is only required when verdict is "downgraded".`

func (v *Verifier) runOpusSingle(ctx context.Context, finding DedupedFinding, rc *resolve.ResolvedContext) (verdict, llm.Usage, error) {
	prompt := buildOpusPrompt(finding, rc)

	model := ModelTierDeep
	if v.opts.VerifierModel != "" {
		model = v.opts.VerifierModel
	}

	resp, usage, err := v.llm.Complete(ctx, llm.Request{
		SystemPrompt: opusSystemPrompt,
		UserPrompt:   prompt,
		JSONOutput:   true,
		Model:        model,
	})
	if err != nil {
		return verdict{Status: StatusConfirmed, Reason: "verification error"}, usage, fmt.Errorf("opus verification: %w", err)
	}

	vd := parseOpusResponse(resp)
	return vd, usage, nil
}

func buildOpusPrompt(finding DedupedFinding, rc *resolve.ResolvedContext) string {
	var b strings.Builder

	fmt.Fprintf(&b, "## Finding to Verify\n")
	fmt.Fprintf(&b, "File: %s, Line: %d\n", finding.File, finding.Line)
	fmt.Fprintf(&b, "Risk: %s, Category: %s\n", finding.Risk, finding.Category)
	fmt.Fprintf(&b, "Consensus: %d/%d agents\n", finding.VoteCount, finding.TotalAgents)
	fmt.Fprintf(&b, "Summary: %s\n", finding.Summary)
	fmt.Fprintf(&b, "Detail: %s\n\n", finding.Detail)

	if finding.CodeExample != "" {
		fmt.Fprintf(&b, "### Suggested fix\n```\n%s\n```\n\n", finding.CodeExample)
	}

	if rc != nil {
		if rc.EnclosingScope != nil {
			fmt.Fprintf(&b, "### Enclosing scope: %s (%s, lines %d-%d)\n```\n%s\n```\n\n",
				rc.EnclosingScope.Symbol.Name,
				rc.EnclosingScope.Symbol.Kind,
				rc.EnclosingScope.Symbol.StartLine,
				rc.EnclosingScope.Symbol.EndLine,
				rc.EnclosingScope.Text)
		}

		for _, ref := range rc.References {
			fmt.Fprintf(&b, "### Referenced: %s (%s in %s, lines %d-%d)\n```\n%s\n```\n\n",
				ref.Symbol.Name,
				ref.Symbol.Kind,
				ref.Symbol.File,
				ref.Symbol.StartLine,
				ref.Symbol.EndLine,
				ref.Text)
		}
	}

	return b.String()
}

type opusResponse struct {
	Verdict      string `json:"verdict"`
	Reason       string `json:"reason"`
	AdjustedRisk Risk   `json:"adjusted_risk"`
}

func parseOpusResponse(response string) verdict {
	// Try direct JSON parse first.
	var resp opusResponse
	if err := json.Unmarshal([]byte(response), &resp); err != nil {
		// Try extracting JSON from markdown code fences.
		cleaned := extractJSON(response)
		if err2 := json.Unmarshal([]byte(cleaned), &resp); err2 != nil {
			// Parse failure → confirmed (safe fallback).
			return verdict{Status: StatusConfirmed, Reason: "verification parse error"}
		}
	}

	switch strings.ToLower(strings.TrimSpace(resp.Verdict)) {
	case "dismissed":
		return verdict{Status: StatusDismissed, Reason: resp.Reason}
	case "downgraded":
		return verdict{Status: StatusDowngraded, Reason: resp.Reason, AdjustedRisk: resp.AdjustedRisk}
	default: // "confirmed" or unknown
		return verdict{Status: StatusConfirmed, Reason: resp.Reason}
	}
}

// extractJSON attempts to extract a JSON object from text that may contain
// markdown code fences or other wrapping.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Try to extract from code fences.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end >= 0 {
			return strings.TrimSpace(s[:end])
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if end := strings.Index(s, "```"); end >= 0 {
			return strings.TrimSpace(s[:end])
		}
	}

	// Try to find a JSON object.
	if start := strings.Index(s, "{"); start >= 0 {
		if end := strings.LastIndex(s, "}"); end > start {
			return s[start : end+1]
		}
	}

	return s
}

// parser.go — parsing and normalization of LLM agent responses.
//
// Extracts structured feedback from raw LLM output using multiple
// strategies (direct JSON, markdown code blocks, JSON marker).
// Also handles field normalization and backward compatibility.
package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rawFinding mirrors Finding but includes a backward-compat "severity" field
// for agents that haven't adopted the new "risk" field yet.
type rawFinding struct {
	Finding
	Severity string `json:"severity"` // backward compat
}

type rawFeedback struct {
	Findings []rawFinding `json:"findings"`
}

func parseFeedback(role, response string) (*Feedback, error) {
	response = strings.TrimSpace(response)

	// Try to extract JSON from the response using a series of strategies.
	// Each returns a parsed rawFeedback or nil if it can't extract one.
	raw, ok := tryParseStrategies(response)
	if !ok {
		return nil, fmt.Errorf("could not extract JSON from response\nRaw: %s", truncateUTF8(response, 200))
	}

	// Normalize and filter findings.
	var findings []Finding
	for i := range raw.Findings {
		f := raw.Findings[i].Finding
		NormalizeFinding(&f, raw.Findings[i].Severity)
		if f.Confidence >= ConfidenceThreshold {
			findings = append(findings, f)
		}
	}

	return &Feedback{Role: role, Findings: findings}, nil
}

// jsonParser is a strategy for extracting rawFeedback from an LLM response.
type jsonParser func(response string) (*rawFeedback, bool)

// tryParseStrategies tries each parsing strategy in order, returning the
// first successful result. Strategies are tried from most specific to most
// lenient: direct JSON, markdown code blocks, JSON marker extraction.
func tryParseStrategies(response string) (*rawFeedback, bool) {
	for _, parse := range []jsonParser{parseDirectJSON, parseCodeBlock, parseJSONMarker} {
		if raw, ok := parse(response); ok {
			return raw, true
		}
	}
	return nil, false
}

// parseDirectJSON tries to unmarshal the entire response as JSON.
func parseDirectJSON(response string) (*rawFeedback, bool) {
	var raw rawFeedback
	if err := json.Unmarshal([]byte(response), &raw); err == nil {
		return &raw, true
	}
	return nil, false
}

// parseCodeBlock extracts JSON from markdown fenced code blocks.
func parseCodeBlock(response string) (*rawFeedback, bool) {
	if !strings.Contains(response, "```") {
		return nil, false
	}
	lines := strings.Split(response, "\n")
	var jsonLines []string
	inBlock := false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inBlock {
				var raw rawFeedback
				if err := json.Unmarshal([]byte(strings.Join(jsonLines, "\n")), &raw); err == nil {
					return &raw, true
				}
				jsonLines = nil
			}
			inBlock = !inBlock
			continue
		}
		if inBlock {
			jsonLines = append(jsonLines, line)
		}
	}
	return nil, false
}

// parseJSONMarker finds a {"findings" substring and extracts the JSON object.
// Tries progressively shorter substrings from the end to handle trailing
// text with extra braces that would cause over-capture.
func parseJSONMarker(response string) (*rawFeedback, bool) {
	start := strings.Index(response, `{"findings"`)
	if start == -1 {
		return nil, false
	}
	const maxAttempts = 10
	attempts := 0
	for end := len(response) - 1; end > start; end-- {
		if response[end] != '}' {
			continue
		}
		var raw rawFeedback
		if err := json.Unmarshal([]byte(response[start:end+1]), &raw); err == nil {
			return &raw, true
		}
		attempts++
		if attempts >= maxAttempts {
			break
		}
	}
	return nil, false
}

// NormalizeFinding sanitizes a finding's fields, applying defaults for
// missing or invalid values. This handles both the new format and backward
// compatibility with agents that still output "severity" instead of "risk".
func NormalizeFinding(f *Finding, severity string) {
	// Risk is normally normalized by UnmarshalJSON during JSON parsing.
	// Handle two additional cases:
	// 1. Backward compat: agents that use "severity" instead of "risk"
	// 2. Direct struct construction (tests) that bypasses UnmarshalJSON
	if f.Risk == "" && severity != "" {
		f.Risk = Risk(strings.ToLower(strings.TrimSpace(severity)))
	}
	if !f.Risk.Valid() {
		f.Risk = RiskInfo
	}

	f.Category = strings.ToLower(strings.TrimSpace(f.Category))
	if !validCategories[f.Category] {
		f.Category = CategoryDesign
	}

	f.Scope = strings.ToLower(strings.TrimSpace(f.Scope))
	if !validScopes[f.Scope] {
		f.Scope = ScopeChanged
	}

	if f.Confidence == 0 {
		f.Confidence = confidenceDefault
	}
	if f.Confidence < 0 {
		f.Confidence = 0
	}
	if f.Confidence > 1 {
		f.Confidence = 1
	}
}

// truncateUTF8 truncates s to at most maxBytes without splitting a UTF-8 character.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk back from maxBytes to avoid splitting a multi-byte rune.
	for maxBytes > 0 && maxBytes < len(s) && s[maxBytes]&0xC0 == 0x80 {
		maxBytes--
	}
	return s[:maxBytes]
}

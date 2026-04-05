package agents

import (
	"encoding/json"
	"strings"
)

// Risk represents the severity level of a finding.
// Using a named type instead of bare strings gives compile-time safety
// and lets us attach methods like Order() directly.
//
// Implements json.Unmarshaler to normalize values at the parse boundary:
// incoming strings are lowercased and trimmed, so "Critical", " WARNING ",
// etc. all unmarshal correctly. Invalid values unmarshal as RiskInfo.
type Risk string

const (
	RiskCritical Risk = "critical"
	RiskWarning  Risk = "warning"
	RiskInfo     Risk = "info"
)

// Order returns a sort key for risk (0=critical, 1=warning, 2=info).
// Lower values indicate higher severity.
func (r Risk) Order() int {
	switch r {
	case RiskCritical:
		return 0
	case RiskWarning:
		return 1
	default:
		return 2
	}
}

// Valid returns true if the risk is one of the known values.
func (r Risk) Valid() bool {
	switch r {
	case RiskCritical, RiskWarning, RiskInfo:
		return true
	default:
		return false
	}
}

// UnmarshalJSON normalizes incoming JSON strings at the parse boundary.
// Lowercases, trims whitespace, and defaults invalid values to RiskInfo.
// This means NormalizeFinding doesn't need to handle Risk normalization —
// by the time it runs, Risk is already a valid, normalized value.
func (r *Risk) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed := Risk(strings.ToLower(strings.TrimSpace(s)))
	if !parsed.Valid() {
		parsed = RiskInfo
	}
	*r = parsed
	return nil
}

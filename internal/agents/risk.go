package agents

// Risk represents the severity level of a finding.
// Using a named type instead of bare strings gives compile-time safety
// and lets us attach methods like Order() directly.
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

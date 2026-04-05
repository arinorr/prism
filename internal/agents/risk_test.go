package agents

import "testing"

func TestRisk_Order(t *testing.T) {
	tests := []struct {
		risk Risk
		want int
	}{
		{RiskCritical, 0},
		{RiskWarning, 1},
		{RiskInfo, 2},
		{Risk(""), 2},        // empty defaults to info-level
		{Risk("unknown"), 2}, // unknown defaults to info-level
	}
	for _, tt := range tests {
		got := tt.risk.Order()
		if got != tt.want {
			t.Errorf("Risk(%q).Order() = %d, want %d", tt.risk, got, tt.want)
		}
	}
}

func TestRisk_Order_Sorting(t *testing.T) {
	// Critical should sort before warning, warning before info.
	if RiskCritical.Order() >= RiskWarning.Order() {
		t.Error("critical should have lower order than warning")
	}
	if RiskWarning.Order() >= RiskInfo.Order() {
		t.Error("warning should have lower order than info")
	}
}

func TestRisk_Valid(t *testing.T) {
	tests := []struct {
		risk Risk
		want bool
	}{
		{RiskCritical, true},
		{RiskWarning, true},
		{RiskInfo, true},
		{Risk(""), false},
		{Risk("unknown"), false},
		{Risk("Critical"), false}, // case-sensitive
		{Risk("WARNING"), false},  // case-sensitive
	}
	for _, tt := range tests {
		got := tt.risk.Valid()
		if got != tt.want {
			t.Errorf("Risk(%q).Valid() = %v, want %v", tt.risk, got, tt.want)
		}
	}
}

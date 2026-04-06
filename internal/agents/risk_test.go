package agents

import (
	"encoding/json"
	"testing"
)

func TestRisk_Order(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	// Critical should sort before warning, warning before info.
	if RiskCritical.Order() >= RiskWarning.Order() {
		t.Error("critical should have lower order than warning")
	}
	if RiskWarning.Order() >= RiskInfo.Order() {
		t.Error("warning should have lower order than info")
	}
}

func TestRisk_Valid(t *testing.T) {
	t.Parallel()
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

func TestRisk_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		json string
		want Risk
	}{
		{"lowercase critical", `{"risk": "critical"}`, RiskCritical},
		{"normalizes whitespace", `{"risk": " WARNING "}`, RiskWarning},
		{"normalizes case", `{"risk": "INFO"}`, RiskInfo},
		{"invalid defaults to info", `{"risk": "BOGUS"}`, RiskInfo},
		{"empty defaults to info", `{"risk": ""}`, RiskInfo},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var f Finding
			if err := json.Unmarshal([]byte(tt.json), &f); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if f.Risk != tt.want {
				t.Errorf("got %q, want %q", f.Risk, tt.want)
			}
		})
	}
}

package config

import "testing"

func TestVerifyEnabled(t *testing.T) {
	tests := []struct {
		name   string
		verify *bool
		want   bool
	}{
		{"nil (not set)", nil, false},
		{"true", BoolPtr(true), true},
		{"false", BoolPtr(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Verify: tt.verify}
			if got := cfg.VerifyEnabled(); got != tt.want {
				t.Errorf("VerifyEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMerge_VerifyFields(t *testing.T) {
	def := Default()
	file := Config{
		Verify:            BoolPtr(true),
		VerifierModel:     "sonnet",
		VerifierBudgetUSD: 0.50,
	}
	cli := Config{}

	merged := Merge(&def, &file, &cli)

	if !merged.VerifyEnabled() {
		t.Error("expected Verify to be true from file config")
	}
	if merged.VerifierModel != "sonnet" {
		t.Errorf("VerifierModel = %q, want sonnet", merged.VerifierModel)
	}
	if merged.VerifierBudgetUSD != 0.50 {
		t.Errorf("VerifierBudgetUSD = %f, want 0.50", merged.VerifierBudgetUSD)
	}
}

func TestMerge_CLIOverridesFileVerify(t *testing.T) {
	def := Default()
	file := Config{Verify: BoolPtr(true)}
	cli := Config{Verify: BoolPtr(false)} // --no-verify

	merged := Merge(&def, &file, &cli)

	if merged.VerifyEnabled() {
		t.Error("CLI --no-verify should override file verify: true")
	}
}

func TestMerge_VerifierBudgetCLIOverride(t *testing.T) {
	def := Default()
	file := Config{VerifierBudgetUSD: 1.00}
	cli := Config{VerifierBudgetUSD: 0.10}

	merged := Merge(&def, &file, &cli)

	if merged.VerifierBudgetUSD != 0.10 {
		t.Errorf("VerifierBudgetUSD = %f, want 0.10 (CLI override)", merged.VerifierBudgetUSD)
	}
}

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || !*p {
		t.Error("BoolPtr(true) should return pointer to true")
	}
	p = BoolPtr(false)
	if p == nil || *p {
		t.Error("BoolPtr(false) should return pointer to false")
	}
}

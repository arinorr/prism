package cmd

import "testing"

func TestParseReviewArgs_VerifyFlag(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantValue bool
		wantSet   bool
	}{
		{"--verify", []string{"--verify", "123"}, true, true},
		{"--no-verify", []string{"--no-verify", "123"}, false, true},
		{"last wins (no-verify)", []string{"--verify", "--no-verify", "123"}, false, true},
		{"neither", []string{"123"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseReviewArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if opts.verify.value != tt.wantValue {
				t.Errorf("verify.value = %v, want %v", opts.verify.value, tt.wantValue)
			}
			if opts.verify.set != tt.wantSet {
				t.Errorf("verify.set = %v, want %v", opts.verify.set, tt.wantSet)
			}
		})
	}
}

func TestParseReviewArgs_VerifierBudget(t *testing.T) {
	opts, err := parseReviewArgs([]string{"--verifier-budget", "0.25", "123"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.verifierBudgetFlag != 0.25 {
		t.Errorf("verifierBudgetFlag = %f, want 0.25", opts.verifierBudgetFlag)
	}
}

func TestParseReviewArgs_VerifierBudgetEquals(t *testing.T) {
	opts, err := parseReviewArgs([]string{"--verifier-budget=0.50", "123"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.verifierBudgetFlag != 0.50 {
		t.Errorf("verifierBudgetFlag = %f, want 0.50", opts.verifierBudgetFlag)
	}
}

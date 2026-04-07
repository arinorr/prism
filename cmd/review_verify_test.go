package cmd

import "testing"

func TestParseReviewArgs_VerifyFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantVerify bool
		wantNoVer  bool
	}{
		{"--verify", []string{"--verify", "123"}, true, false},
		{"--no-verify", []string{"--no-verify", "123"}, false, true},
		{"both (no-verify wins)", []string{"--verify", "--no-verify", "123"}, true, true},
		{"neither", []string{"123"}, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := parseReviewArgs(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if opts.verify != tt.wantVerify {
				t.Errorf("verify = %v, want %v", opts.verify, tt.wantVerify)
			}
			if opts.noVerify != tt.wantNoVer {
				t.Errorf("noVerify = %v, want %v", opts.noVerify, tt.wantNoVer)
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

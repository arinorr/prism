package cmd

import (
	"strings"
	"testing"

	"github.com/arinorr/prism/internal/agents"
	"github.com/arinorr/prism/internal/config"
)

// TestResolveTier_Defaults_NoOverrides — no tier specified anywhere falls
// back to TierDeep defaults: per-role models, cross-refs + scope hints on,
// verify off.
func TestResolveTier_Defaults_NoOverrides(t *testing.T) {
	merged := &config.Config{}
	opts := &reviewOptions{}

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Model != "" {
		t.Errorf("Model = %q, want empty (per-role)", rc.Model)
	}
	if !rc.CrossRefs || !rc.ScopeHints {
		t.Errorf("expected CrossRefs and ScopeHints true (TierDeep), got %+v", rc)
	}
	if rc.Verify {
		t.Error("expected Verify false (TierDeep), got true")
	}
}

// TestResolveTier_EachTier verifies every named tier expands to its
// expected ReviewConfig defaults.
func TestResolveTier_EachTier(t *testing.T) {
	tests := []struct {
		tier       string
		wantModel  string
		wantCross  bool
		wantVerify bool
	}{
		{"quick", agents.ModelTierFast, false, false},
		{"standard", agents.ModelTierStandard, false, false},
		{"deep", "", true, false},
		{"thorough", "", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			merged := &config.Config{}
			opts := &reviewOptions{tierFlag: tt.tier}

			rc, err := resolveTier(merged, opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rc.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", rc.Model, tt.wantModel)
			}
			if rc.CrossRefs != tt.wantCross {
				t.Errorf("CrossRefs = %v, want %v", rc.CrossRefs, tt.wantCross)
			}
			if rc.Verify != tt.wantVerify {
				t.Errorf("Verify = %v, want %v", rc.Verify, tt.wantVerify)
			}
		})
	}
}

// TestResolveTier_InvalidTierFromConfig — invalid tier in the config file
// (validateOptions only catches the CLI flag) returns an error.
func TestResolveTier_InvalidTierFromConfig(t *testing.T) {
	merged := &config.Config{Tier: "turbo"}
	opts := &reviewOptions{}

	_, err := resolveTier(merged, opts)
	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
	if !strings.Contains(err.Error(), "turbo") {
		t.Errorf("error should mention the bad tier name, got: %v", err)
	}
}

// TestResolveTier_CLITierWinsOverConfig — when CLI and config both set tier,
// CLI wins (matches the "last write wins" precedence promised in the comment).
func TestResolveTier_CLITierWinsOverConfig(t *testing.T) {
	merged := &config.Config{Tier: "standard"}
	opts := &reviewOptions{tierFlag: "quick"}

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Quick tier sets Model=Haiku; standard would have set it to Sonnet.
	if rc.Model != agents.ModelTierFast {
		t.Errorf("Model = %q, want %q (quick should win over standard)", rc.Model, agents.ModelTierFast)
	}
}

// TestResolveTier_ConfigOverridesTierDefault — when config file sets a model
// but tier is deep (which has no model default), the config model takes effect.
func TestResolveTier_ConfigOverridesTierDefault(t *testing.T) {
	merged := &config.Config{Model: "haiku"}
	opts := &reviewOptions{} // tier defaults to deep

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Model != "haiku" {
		t.Errorf("Model = %q, want %q", rc.Model, "haiku")
	}
	// Deep tier features should still be enabled.
	if !rc.CrossRefs {
		t.Error("expected CrossRefs true from deep tier")
	}
}

// TestResolveTier_CLICrossRefsTrueOverridesConfig — boolFlag.set distinguishes
// "explicitly --cross-refs" from "flag absent". Setting it overrides config.
func TestResolveTier_CLICrossRefsTrueOverridesConfig(t *testing.T) {
	falseVal := false
	merged := &config.Config{CrossRefs: &falseVal}
	opts := &reviewOptions{
		tierFlag:  "quick", // quick has CrossRefs=false by default
		crossRefs: boolFlag{value: true, set: true},
	}

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rc.CrossRefs {
		t.Error("expected CrossRefs true (CLI override should win)")
	}
	if !rc.ScopeHints {
		t.Error("expected ScopeHints true (always tracks CrossRefs)")
	}
}

// TestResolveTier_CLINoCrossRefsOverridesConfig — --no-cross-refs (set=true,
// value=false) must override a config that enabled cross-refs.
func TestResolveTier_CLINoCrossRefsOverridesConfig(t *testing.T) {
	trueVal := true
	merged := &config.Config{CrossRefs: &trueVal}
	opts := &reviewOptions{
		tierFlag:  "deep", // deep has CrossRefs=true by default
		crossRefs: boolFlag{value: false, set: true},
	}

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.CrossRefs {
		t.Error("expected CrossRefs false (--no-cross-refs should win)")
	}
	if rc.ScopeHints {
		t.Error("expected ScopeHints false (tracks CrossRefs)")
	}
}

// TestResolveTier_UnsetBoolFlagDoesNotClobberTier — when a CLI flag wasn't
// passed (set=false), the tier default must survive intact.
func TestResolveTier_UnsetBoolFlagDoesNotClobberTier(t *testing.T) {
	opts := &reviewOptions{
		tierFlag: "thorough", // thorough sets Verify=true by default
		verify:   boolFlag{}, // unset
	}

	rc, err := resolveTier(&config.Config{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rc.Verify {
		t.Error("expected Verify true (thorough tier default), got false — unset boolFlag clobbered tier")
	}
}

// TestResolveTier_ConfigVerifyOverridesTier — config Verify *bool overrides
// tier default when the CLI flag isn't set.
func TestResolveTier_ConfigVerifyOverridesTier(t *testing.T) {
	trueVal := true
	merged := &config.Config{Verify: &trueVal}
	opts := &reviewOptions{tierFlag: "quick"} // quick has Verify=false by default

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rc.Verify {
		t.Error("expected Verify true (config override), got false")
	}
}

// TestResolveTier_CLIVerifyOverridesConfig — last-write-wins: CLI verify
// boolFlag set=true overrides config Verify.
func TestResolveTier_CLIVerifyOverridesConfig(t *testing.T) {
	trueVal := true
	merged := &config.Config{Verify: &trueVal}
	opts := &reviewOptions{
		tierFlag: "deep",
		verify:   boolFlag{value: false, set: true}, // --no-verify
	}

	rc, err := resolveTier(merged, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rc.Verify {
		t.Error("expected Verify false (--no-verify should win over config)")
	}
}

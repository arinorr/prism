package agents

import "testing"

func TestTier_Valid(t *testing.T) {
	tests := []struct {
		tier Tier
		want bool
	}{
		{TierQuick, true},
		{TierStandard, true},
		{TierDeep, true},
		{TierThorough, true},
		{"turbo", false},
		{"", false},
		{"QUICK", false}, // case-sensitive
	}
	for _, tt := range tests {
		if got := tt.tier.Valid(); got != tt.want {
			t.Errorf("Tier(%q).Valid() = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

func TestTier_Defaults(t *testing.T) {
	// Quick: all Haiku, no context features.
	q := TierQuick.Defaults()
	if q.Model != ModelTierFast {
		t.Errorf("quick model = %q, want %q", q.Model, ModelTierFast)
	}
	if q.CrossRefs || q.ScopeHints || q.Verify {
		t.Errorf("quick should have no context features, got crossRefs=%v scopeHints=%v verify=%v",
			q.CrossRefs, q.ScopeHints, q.Verify)
	}

	// Standard: all Sonnet, no context features.
	s := TierStandard.Defaults()
	if s.Model != ModelTierStandard {
		t.Errorf("standard model = %q, want %q", s.Model, ModelTierStandard)
	}
	if s.CrossRefs || s.ScopeHints || s.Verify {
		t.Errorf("standard should have no context features")
	}

	// Deep: per-role (empty model), cross-refs + scope hints, no verify.
	d := TierDeep.Defaults()
	if d.Model != "" {
		t.Errorf("deep model = %q, want empty (per-role)", d.Model)
	}
	if !d.CrossRefs || !d.ScopeHints {
		t.Errorf("deep should have crossRefs and scopeHints enabled")
	}
	if d.Verify {
		t.Errorf("deep should not have verify enabled")
	}

	// Thorough: per-role, cross-refs + scope hints + verify.
	th := TierThorough.Defaults()
	if th.Model != "" {
		t.Errorf("thorough model = %q, want empty (per-role)", th.Model)
	}
	if !th.CrossRefs || !th.ScopeHints || !th.Verify {
		t.Errorf("thorough should have all features enabled")
	}
}

// TestTier_Defaults_UnknownReturnsDeep documents the defense-in-depth
// fallback behavior of Tier.Defaults() for unknown tier values. In the
// production path, resolveTier rejects unknown tiers before Defaults() is
// reached, so this fallback is only reachable via direct callers.
func TestTier_Defaults_UnknownReturnsDeep(t *testing.T) {
	got := Tier("unknown").Defaults()
	want := TierDeep.Defaults()
	if got != want {
		t.Errorf("unknown tier defaults = %+v, want deep defaults %+v", got, want)
	}
}

func TestTier_Description(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{TierQuick, "fast scan, all Haiku"},
		{TierStandard, "balanced, all Sonnet"},
		{TierDeep, "per-role models, codebase context"},
		{TierThorough, "per-role models, codebase context, false positive filtering"},
	}
	for _, tt := range tests {
		if got := tt.tier.Description(); got != tt.want {
			t.Errorf("Tier(%q).Description() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

// TestTier_Description_UnknownReturnsDeep documents the defense-in-depth
// fallback behavior of Tier.Description() for unknown tier values. See
// TestTier_Defaults_UnknownReturnsDeep for the same caveat.
func TestTier_Description_UnknownReturnsDeep(t *testing.T) {
	got := Tier("unknown").Description()
	want := TierDeep.Description()
	if got != want {
		t.Errorf("unknown tier description = %q, want %q", got, want)
	}
}

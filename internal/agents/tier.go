package agents

// Tier is a named preset that expands into ReviewConfig defaults.
// Tiers are sugar for flag combinations — not a separate system.
// A tier sets defaults; explicit CLI flags adjust from there.
type Tier string

const (
	TierQuick    Tier = "quick"
	TierStandard Tier = "standard"
	TierDeep     Tier = "deep"
	TierThorough Tier = "thorough"
)

// Valid returns true if the tier is one of the known values.
func (t Tier) Valid() bool {
	switch t {
	case TierQuick, TierStandard, TierDeep, TierThorough:
		return true
	}
	return false
}

// Defaults returns the flag defaults for this tier.
func (t Tier) Defaults() ReviewConfig {
	switch t {
	case TierQuick:
		return ReviewConfig{Model: ModelTierFast}
	case TierStandard:
		return ReviewConfig{Model: ModelTierStandard}
	case TierDeep:
		return ReviewConfig{CrossRefs: true, ScopeHints: true}
	case TierThorough:
		return ReviewConfig{CrossRefs: true, ScopeHints: true, Verify: true}
	default:
		return ReviewConfig{CrossRefs: true, ScopeHints: true}
	}
}

// Description returns a user-facing one-liner for help text and prompts.
func (t Tier) Description() string {
	switch t {
	case TierQuick:
		return "fast scan, all Haiku"
	case TierStandard:
		return "balanced, all Sonnet"
	case TierDeep:
		return "per-role models, codebase context"
	case TierThorough:
		return "per-role models, codebase context, false positive filtering"
	default:
		return "per-role models, codebase context"
	}
}

// ReviewConfig is the resolved runtime configuration for a review.
// After tier + config + CLI resolution, every field has a concrete value.
// This is NOT the file-based config (config.Config) — it's the final
// resolved state that the orchestrator uses.
type ReviewConfig struct {
	// Model is the model override for all agents.
	// Empty string means per-role defaults (each agent uses its role's model).
	Model      string
	CrossRefs  bool
	ScopeHints bool // follows CrossRefs — not independently settable via CLI.
	Verify     bool
}

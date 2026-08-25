package agent

// Mode selects the engine's turn posture: which system-prompt variant runs
// and how far the built-in tool set reaches.
type Mode string

const (
	// ModeBuild is the full-toolset posture: every built-in tool and an
	// interactive permission gate.
	ModeBuild Mode = "build"
	// ModePlan is read-only planning: no write/edit tools and a plan appendix
	// in the system prompt. Pair it with a readonly permission policy at the
	// controller so mutating bash folds to the allowlist too.
	ModePlan Mode = "plan"
	// ModeUsePlan hands control to the model: an approved plan gates every
	// tool call by an in_progress plan step, and misses are denied.
	ModeUsePlan Mode = "useplan"
)

// normalizeMode maps unknown or empty values to ModeUsePlan: the primary
// posture is to run under an approved plan.
func normalizeMode(m Mode) Mode {
	switch m {
	case ModeBuild:
		return ModeBuild
	case ModePlan:
		return ModePlan
	case ModeUsePlan:
		return ModeUsePlan
	}
	return ModeUsePlan
}

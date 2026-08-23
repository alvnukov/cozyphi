package agent

// Mode selects the engine's turn posture: which system-prompt variant runs
// and how far the built-in tool set reaches.
type Mode string

const (
	// ModeBuild is the default posture: full tool set, interactive gate.
	ModeBuild Mode = "build"
	// ModePlan is read-only planning: no write/edit tools and a plan appendix
	// in the system prompt. Pair it with a readonly permission policy at the
	// controller so mutating bash folds to the allowlist too.
	ModePlan Mode = "plan"
)

// normalizeMode maps unknown or empty values to ModeBuild.
func normalizeMode(m Mode) Mode {
	if m == ModePlan {
		return ModePlan
	}
	return ModeBuild
}

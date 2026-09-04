package session

import (
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// NoModelLabel is what every model display shows when the session has no
// model at all: a placeholder beats an empty name, which reads as a
// rendering bug rather than a missing configuration.
const NoModelLabel = "no model"

// ParseModelRef splits a "name:effort" model reference into its parts —
// the convention the plan store, the pickers and the remembered picks all
// share. A suffix outside the reasoning effort ladder is not an effort:
// the whole reference stays the model name, so Ollama-style tags and
// typos never lose half a model name. The split prefers the last colon,
// mirroring the legacy picker resolution.
func ParseModelRef(ref string) (name, effort string) {
	i := strings.LastIndex(ref, ":")
	if i < 0 || i == len(ref)-1 {
		return ref, ""
	}
	parsed, valid := llm.ParseReasoningEffort(ref[i+1:])
	if !valid || parsed == "" {
		return ref, ""
	}
	return ref[:i], string(parsed)
}

// FormatModelRef packs a model and effort into the shared "name:effort"
// reference; an empty effort packs to the bare name. It is the exact
// inverse of ParseModelRef, so a reference round-trips through the plan
// store unchanged.
func FormatModelRef(name, effort string) string {
	if effort == "" {
		return name
	}
	return name + ":" + effort
}

// ModelLabel renders a model the way every info line does: the name,
// plus " · <effort>" only when one is selected. A missing model renders
// as the placeholder, not an empty string.
func ModelLabel(name, effort string) string {
	if name == "" {
		return NoModelLabel
	}
	if effort == "" {
		return name
	}
	return name + " · " + effort
}

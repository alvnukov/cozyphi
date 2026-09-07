package job

import (
	"fmt"
	"strings"
)

// Role selects a sub-agent capability profile (tools + permission posture).
type Role string

const (
	// RoleExplore is read-only codebase search / structure (default).
	RoleExplore Role = "explore"
	// RoleWorker may read and write; for planned, independent change blocks.
	RoleWorker Role = "worker"
	// RoleReview is read-only + bash for diffs / checks (no edits).
	RoleReview Role = "review"
	// RoleWebReader is the quarantine reader for untrusted web text: one
	// tool-less model call that answers a question from a page fragment.
	//
	// It is deliberately absent from Roles() and refused by ParseRole. It is
	// not a job anyone spawns — it is a defense the web tool runs — and a
	// role that could be spawned could be pointed at any prompt, which is
	// the opposite of what a reader with no tools is for.
	RoleWebReader Role = "web-reader"
)

// Roles lists every sub-agent role in canonical order — the order config
// warnings, settings rows, and any role-indexed UI address by.
func Roles() []Role {
	return []Role{RoleExplore, RoleWorker, RoleReview}
}

// ParseRole normalizes a role string. Empty → explore.
func ParseRole(s string) (Role, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(RoleExplore):
		return RoleExplore, nil
	case string(RoleWorker):
		return RoleWorker, nil
	case string(RoleReview):
		return RoleReview, nil
	case string(RoleWebReader):
		return "", fmt.Errorf(
			"%w: role %q is the web tool's internal quarantine reader and cannot be spawned", ErrInvalid, s)
	default:
		return "", fmt.Errorf("%w: unknown role %q (want explore|worker|review)", ErrInvalid, s)
	}
}

// NormalizeModels is the single interpretation of agents.models pins:
// every key must be a known role, an empty model name means "inherit" and
// is dropped, and a nil or empty input normalizes to nil. The config loader
// and the settings writer both go through it, so what loads is what saves.
func NormalizeModels(models map[string]string) (map[string]string, error) {
	if len(models) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(models))
	for role, name := range models {
		r, err := ParseRole(role)
		if err != nil {
			return nil, err
		}
		if name = strings.TrimSpace(name); name != "" {
			out[string(r)] = name
		}
	}
	return out, nil
}

// NormalizeRole returns explore for empty/unknown without error (legacy meta).
func NormalizeRole(s string) Role {
	r, err := ParseRole(s)
	if err != nil {
		return RoleExplore
	}
	return r
}

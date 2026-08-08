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
)

// ParseRole normalizes a role string. Empty → explore.
func ParseRole(s string) (Role, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(RoleExplore):
		return RoleExplore, nil
	case string(RoleWorker):
		return RoleWorker, nil
	case string(RoleReview):
		return RoleReview, nil
	default:
		return "", fmt.Errorf("%w: unknown role %q (want explore|worker|review)", ErrInvalid, s)
	}
}

// NormalizeRole returns explore for empty/unknown without error (legacy meta).
func NormalizeRole(s string) Role {
	r, err := ParseRole(s)
	if err != nil {
		return RoleExplore
	}
	return r
}

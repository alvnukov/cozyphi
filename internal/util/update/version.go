package update

import (
	"strconv"
	"strings"
)

// versionOnly strips build metadata appended at build time
// (e.g. "v0.2.0 (abc1234, 2026-05-12)" -> "v0.2.0").
func versionOnly(v string) string {
	if i := strings.IndexAny(v, " ("); i > 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

// splitVersion parses a dotted version string into integers.
func splitVersion(s string) []int {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexAny(s, " (-"); i > 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}

// VersionLess reports whether a < b for dotted semver-like tags (e.g. "v0.2.0").
func VersionLess(a, b string) bool {
	as := splitVersion(a)
	bs := splitVersion(b)
	for i := range 3 {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

// IsDevBuild reports whether update is disabled for this version string.
func IsDevBuild(version string) bool {
	v := strings.TrimSpace(versionOnly(version))
	return v == "" || v == "dev" || v == "0.0.0" || v == "v0.0.0"
}

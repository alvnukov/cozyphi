// Package redact masks known secret shapes before plan text is persisted or
// projected. The durable plan is model-authored: it must never become a home
// for credentials that rode in through tool output or prose, and every
// downstream surface (projection, full view, sidebar, receipt, audit) inherits
// the mask rather than reimplementing it.
package redact

import "regexp"

// Marker replaces every matched secret span. It matches no rule itself, so
// Redact is idempotent — re-masking already-masked text is a no-op, which the
// load path and the projection renderer both rely on.
const Marker = "[REDACTED]"

// rule pairs one secret shape with its replacement: most rules replace the
// whole span, assignment rules keep the key name so the line stays readable.
type rule struct {
	pattern     *regexp.Regexp
	replacement string
}

var pack = []rule{
	// AWS access key id, AWS secret assignment, GitHub PAT, OpenAI-style key.
	// The shapes are deliberately narrow: hyphens inside the credential body
	// would let kebab-case slugs ("task-sk-v2-…") false-positive, so only the
	// documented prefix shapes match.
	{regexp.MustCompile(`AKIA[A-Z0-9]{16}`), Marker},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`), Marker},
	{regexp.MustCompile(`sk-(proj-)?[A-Za-z0-9]{20,}`), Marker},
	// Bearer credentials: keep the scheme, mask the credential.
	{regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._+/=-]{12,}`), `${1}` + Marker},
	// Credential-shaped assignments: keep the key, mask the value. The value
	// charset excludes spaces and hyphens so hyphenated prose values never
	// match; the key set mirrors hooks' sensitive env vocabulary.
	{
		regexp.MustCompile(
			`(?i)\b([A-Z0-9_]*(?:API_KEY|SECRET|TOKEN|PASSWORD|PRIVATE_KEY|CREDENTIAL)[A-Z0-9_]*)\s*=\s*("[^"]{8,}"|[A-Za-z0-9_./+=]{12,})`,
		),
		`${1}=` + Marker,
	},
}

// Redact returns s with every known secret shape replaced by Marker. It is
// conservative by design: only strong prefixed shapes and sensitive-key
// assignments match, so ordinary prose — commit ids, file names, kebab slugs —
// survives untouched.
func Redact(s string) string {
	for _, r := range pack {
		s = r.pattern.ReplaceAllString(s, r.replacement)
	}
	return s
}

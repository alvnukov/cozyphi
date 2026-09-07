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
	// AWS access key id and GitHub PAT. Both bodies are a fixed length in the
	// documented form, but the count is a lower bound rather than an equality:
	// an exact count masks its own length and hands the remainder of a longer
	// token back to the reader, which is a leak dressed as a mask.
	{regexp.MustCompile(`AKIA[A-Z0-9]{16,}`), Marker},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`), Marker},
	// Anthropic. Its key body carries hyphens ("sk-ant-api03-…"), so the rule
	// below — which forbids them — never saw this product's own credential.
	// The permission to carry hyphens is bought by spelling the prefix out:
	// it is the prefix, not the body charset, that keeps a kebab-case slug
	// ("task-sk-v2-…") from matching. The leading word boundary is what stops
	// prose from lending the prefix, as "risk-ant-…" otherwise would.
	{regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), Marker},
	// OpenAI and the sk- compatibles (deepseek). Every current form spells its
	// kind in the prefix and keeps the body itself hyphen-free, so the body
	// stays narrow and only the known kinds are admitted.
	{regexp.MustCompile(`sk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9]{20,}`), Marker},
	// Groq, which is what `voice.stt.api_key` holds, and a Google API key,
	// which is what web search sends — the latter travels in a query string,
	// so it reaches prose by way of a URL rather than a configuration file.
	{regexp.MustCompile(`gsk_[A-Za-z0-9]{20,}`), Marker},
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{35,}`), Marker},
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

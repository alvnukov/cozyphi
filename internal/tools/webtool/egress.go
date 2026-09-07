package webtool

import (
	"fmt"
	"os"
	"strings"

	"github.com/alvnukov/cozy-tools/security"
)

// MaxEgressLength bounds a URL or query before it is allowed near the
// network. 2048 is the practical browser limit for a URL, and a longer one in
// a model-authored argument is far more likely to be a payload — a base64
// blob, a stuffed query string — than an address anyone meant to visit.
const MaxEgressLength = 2048

// minSecretLength is the shortest env value treated as a secret. Short values
// ("1", "true", a two-letter locale) appear in ordinary URLs by coincidence,
// and masking those would refuse honest fetches.
const minSecretLength = 8

// secretNameParts are the substrings that mark an environment variable as
// carrying a credential. This is a heuristic and is documented as one: it is
// tuned to catch the obvious carriers, not to be complete.
var secretNameParts = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "PRIVATE"}

// secretNameSuffixes name variables whose value is a location, not a
// credential — SSH_AUTH_SOCK, GOOGLE_APPLICATION_CREDENTIALS_FILE. Masking a
// path would refuse URLs that merely mention a directory.
var secretNameSuffixes = []string{"_SOCK", "_FILE", "_PATH", "_DIR", "_HOME"}

// SecretMask builds the mask the egress check scans URLs and queries with:
// every environment value whose variable name looks like a credential, plus
// any values the host passes explicitly (configured model API keys, the
// Google CSE key resolved from its env var name).
//
// The mask exists to catch the shape of an exfiltration attempt — a page that
// talks the model into putting a token in a query string — before the request
// leaves. It is a heuristic in both directions: an oddly named secret is
// missed, and a credential-looking value that legitimately belongs in a URL
// is refused.
func SecretMask(extra ...string) *security.Mask {
	mask := security.NewMask()
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || len(value) < minSecretLength || !secretVarName(name) {
			continue
		}
		mask.AddNamed(name, value)
	}
	for _, value := range extra {
		if len(strings.TrimSpace(value)) >= minSecretLength {
			mask.Add(strings.TrimSpace(value))
		}
	}
	return mask
}

func secretVarName(name string) bool {
	upper := strings.ToUpper(name)
	for _, suffix := range secretNameSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return false
		}
	}
	for _, part := range secretNameParts {
		if strings.Contains(upper, part) {
			return true
		}
	}
	return false
}

// checkEgress is the last thing that happens before a value is handed to the
// network. It refuses rather than sanitizes: a masked URL would be a
// different URL, and quietly fetching a different address than the model
// asked for is worse than saying no.
func checkEgress(mask *security.Mask, kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("web: %s is required", kind)
	}
	if len(value) > MaxEgressLength {
		return fmt.Errorf("web: %s is %d bytes, over the %d-byte limit; shorten it or fetch a narrower address",
			kind, len(value), MaxEgressLength)
	}
	if mask == nil {
		return nil
	}
	if masked := mask.Apply(value); masked != value {
		return fmt.Errorf("web: refused to send %s — it contains a secret from this environment (%s). "+
			"Credentials must never travel in a URL or a search query", kind, firstMaskToken(masked))
	}
	return nil
}

// firstMaskToken names the secret that tripped the check without repeating
// the secret. security.Mask renders a named value as [HELPER_SECRET:NAME].
func firstMaskToken(masked string) string {
	const open = "[HELPER_SECRET:"
	_, rest, found := strings.Cut(masked, open)
	if !found {
		return "unnamed value"
	}
	name, _, found := strings.Cut(rest, "]")
	if !found {
		return "unnamed value"
	}
	return name
}

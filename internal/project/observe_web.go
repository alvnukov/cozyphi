package project

import (
	"strings"

	cozyconfig "github.com/alvnukov/cozy-tools/config"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// ObserveWebPolicy projects one web policy into what the harness may say
// about it. It is the seam every web observation goes through, so what is
// publishable is decided once, here, where the whole policy is in scope.
//
// It is an allowlist by construction: diag.WebPolicyFacts has no member for a
// host, a scheme, a content type, a cache path, a user-agent string, a search
// address, a custom-search id or an API key, so every list is reduced to a
// count and every string to a presence before it leaves this function. A rule
// literal cannot reach the view because there is nowhere in the view for one
// to sit.
//
// Nothing here resolves a host, opens the cache, reads an environment
// variable or contacts anything: the policy is data, and this is a reading
// of it.
func ObserveWebPolicy(p cozyconfig.WebPolicy) diag.WebPolicyFacts {
	return diag.WebPolicyFacts{
		Enabled:  p.IsEnabled(),
		CacheSet: strings.TrimSpace(p.CacheDir) != "",
		// nil and empty are two different answers: the loader assigns a list
		// only when the file wrote one, so a nil list is the library's own
		// set in force and an empty one is a list somebody emptied.
		Schemes:                len(p.AllowedSchemes),
		SchemesConfigured:      p.AllowedSchemes != nil,
		AllowedHosts:           len(p.AllowedHosts),
		DeniedHosts:            len(p.DeniedHosts),
		ContentTypes:           len(p.AcceptedContentTypes),
		ContentTypesConfigured: p.AcceptedContentTypes != nil,
		MaxSourceBytes:         p.MaxSourceBytes,
		TimeoutSeconds:         p.TimeoutSeconds,
		MaxRedirects:           p.MaxRedirects,
		CustomUserAgent:        strings.TrimSpace(p.UserAgent) != "",
		SearchProvider:         strings.TrimSpace(p.SearchProvider),
		MaxSearchResults:       p.MaxSearchResults,
		SearchEndpoint:         strings.TrimSpace(p.SearchURL) != "",
		CSEEndpoint:            strings.TrimSpace(p.GoogleCSEURL) != "",
		CSEID:                  strings.TrimSpace(p.GoogleCSEID) != "",
		CredentialEnv:          strings.TrimSpace(p.GoogleAPIKeyEnv) != "",
	}
}

// ObserveWeb projects the resolved web: section: the policy above, plus the
// two things that are cozyphi's own rather than the library's — the
// quarantine mode, and where the cache directory came from.
//
// defaultCacheDir is what this installation's layout fixes, and it is passed
// in rather than looked up so that the comparison is against the layout this
// configuration was finalized under. It is only ever compared: the directory
// itself does not travel, because a cache path is a path somebody wrote.
func ObserveWeb(cfg WebConfig, defaultCacheDir string) diag.WebConfigFacts {
	return diag.WebConfigFacts{
		Known:      true,
		Policy:     ObserveWebPolicy(cfg.Policy),
		Quarantine: string(cfg.Quarantine),
		Cache:      webCacheKind(cfg.Policy.CacheDir, defaultCacheDir),
	}
}

// webCacheKind says where fetched documents land without saying which
// directory that is. An unset one is the case that matters most: it leaves
// the web tool out entirely, however enabled the policy is.
func webCacheKind(dir, def string) diag.WebCacheKind {
	dir, def = strings.TrimSpace(dir), strings.TrimSpace(def)
	switch dir {
	case "":
		return diag.WebCacheUnset
	case def:
		return diag.WebCacheDefault
	default:
		return diag.WebCacheCustom
	}
}

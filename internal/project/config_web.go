package project

import (
	"fmt"
	"strings"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
)

// WebQuarantine selects how fetched page text reaches the model.
type WebQuarantine string

// Web quarantine modes.
const (
	// WebQuarantineReader is the default: read and find hand the bounded
	// fragment to a tool-less child model call, and only its answer reaches
	// the session.
	WebQuarantineReader WebQuarantine = "reader"
	// WebQuarantineOff hands the bounded fragment straight to the session,
	// still inside the untrusted frame. It is the user's own choice to trade
	// the quarantine for fidelity.
	WebQuarantineOff WebQuarantine = "off"
)

// WebConfig is the resolved `web:` section. The library policy is the data
// cozy-tools consumes; Quarantine is cozyphi's own defense layer and has no
// meaning inside the library.
type WebConfig struct {
	Policy     cozyconfig.WebPolicy
	Quarantine WebQuarantine
}

// Enabled reports whether the web tool is registered at all.
func (w WebConfig) Enabled() bool { return w.Policy.IsEnabled() }

// webFileConfig mirrors the `web:` YAML section. Every key is a pointer or a
// list so an absent key keeps the library's own default instead of writing a
// zero over it.
//
// google_api_key exists here only to be refused: an API key that travels in a
// request query string must not sit in a world-readable config file, so the
// loader drops a literal and names google_api_key_env instead.
type webFileConfig struct {
	Enabled              *bool       `yaml:"enabled"`
	CacheDir             *string     `yaml:"cache_dir"`
	MaxSourceBytes       *int64      `yaml:"max_source_bytes"`
	TimeoutSeconds       *int        `yaml:"timeout_seconds"`
	MaxRedirects         *int        `yaml:"max_redirects"`
	AllowedSchemes       *stringList `yaml:"allowed_schemes"`
	AllowedHosts         *stringList `yaml:"allowed_hosts"`
	DeniedHosts          *stringList `yaml:"denied_hosts"`
	AcceptedContentTypes *stringList `yaml:"accepted_content_types"`
	UserAgent            *string     `yaml:"user_agent"`
	SearchProvider       *string     `yaml:"search_provider"`
	SearchURL            *string     `yaml:"search_url"`
	MaxSearchResults     *int        `yaml:"max_search_results"`
	GoogleCSEID          *string     `yaml:"google_cse_id"`
	GoogleAPIKeyEnv      *string     `yaml:"google_api_key_env"`
	GoogleAPIKey         *string     `yaml:"google_api_key"`
	GoogleCSEURL         *string     `yaml:"google_cse_url"`
	Quarantine           *string     `yaml:"quarantine"`
	Allow                *stringList `yaml:"allow"`
}

// defaultWebConfig is what a config file with no `web:` section runs on:
// network access off. The library's own default is on, but a capability that
// reaches the open internet is not something an existing config should gain by
// upgrading — writing the section is the opt-in.
//
// CacheDir stays empty here and is filled from the global layout in
// finalizeConfig, which is the only place that knows where ~/.cozyphi is.
func defaultWebConfig() WebConfig {
	off := false
	return WebConfig{
		Policy:     cozyconfig.WebPolicy{Enabled: &off},
		Quarantine: WebQuarantineReader,
	}
}

// applyWeb merges the file's web block over the defaults and returns the
// host allow-list for the permission policy plus any load-time warnings.
// An unknown quarantine mode is an error: silently falling back to the
// stricter mode would be fine, to the laxer one would not, and guessing which
// the user meant is not the loader's call.
func applyWeb(w *WebConfig, raw *webFileConfig) (allow, warnings []string, err error) {
	if raw == nil {
		return nil, nil, nil
	}
	p := &w.Policy
	// A `web:` section in the file is the opt-in, so writing one hands the
	// enabled flag back to the library default (on) unless the file says
	// otherwise.
	p.Enabled = nil
	if raw.Enabled != nil {
		enabled := *raw.Enabled
		p.Enabled = &enabled
	}
	if raw.CacheDir != nil {
		p.CacheDir = strings.TrimSpace(*raw.CacheDir)
	}
	if raw.MaxSourceBytes != nil {
		p.MaxSourceBytes = *raw.MaxSourceBytes
	}
	if raw.TimeoutSeconds != nil {
		p.TimeoutSeconds = *raw.TimeoutSeconds
	}
	if raw.MaxRedirects != nil {
		p.MaxRedirects = *raw.MaxRedirects
	}
	if raw.AllowedSchemes != nil {
		p.AllowedSchemes = *raw.AllowedSchemes
	}
	if raw.AllowedHosts != nil {
		p.AllowedHosts = *raw.AllowedHosts
	}
	if raw.DeniedHosts != nil {
		p.DeniedHosts = *raw.DeniedHosts
	}
	if raw.AcceptedContentTypes != nil {
		p.AcceptedContentTypes = *raw.AcceptedContentTypes
	}
	if raw.UserAgent != nil {
		p.UserAgent = strings.TrimSpace(*raw.UserAgent)
	}
	if raw.SearchProvider != nil {
		p.SearchProvider = strings.TrimSpace(*raw.SearchProvider)
	}
	if raw.SearchURL != nil {
		p.SearchURL = strings.TrimSpace(*raw.SearchURL)
	}
	if raw.MaxSearchResults != nil {
		p.MaxSearchResults = *raw.MaxSearchResults
	}
	if raw.GoogleCSEID != nil {
		p.GoogleCSEID = strings.TrimSpace(*raw.GoogleCSEID)
	}
	if raw.GoogleAPIKeyEnv != nil {
		p.GoogleAPIKeyEnv = strings.TrimSpace(*raw.GoogleAPIKeyEnv)
	}
	if raw.GoogleCSEURL != nil {
		p.GoogleCSEURL = strings.TrimSpace(*raw.GoogleCSEURL)
	}
	// The key itself never survives the loader, whatever the file says.
	if raw.GoogleAPIKey != nil && strings.TrimSpace(*raw.GoogleAPIKey) != "" {
		warnings = append(warnings,
			"web.google_api_key is ignored: the key travels in a request query string and must not sit in "+
				"config.yaml — name the environment variable in web.google_api_key_env instead")
	}
	p.GoogleAPIKey = ""
	if raw.Quarantine != nil {
		mode, parseErr := ParseWebQuarantine(*raw.Quarantine)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		w.Quarantine = mode
	}
	if raw.Allow != nil {
		allow = *raw.Allow
	}
	return allow, warnings, nil
}

// ParseWebQuarantine normalizes the web.quarantine setting. Empty reads as
// reader, the default.
func ParseWebQuarantine(s string) (WebQuarantine, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(WebQuarantineReader):
		return WebQuarantineReader, nil
	case string(WebQuarantineOff):
		return WebQuarantineOff, nil
	default:
		return "", fmt.Errorf("web.quarantine: unknown mode %q (use reader or off)", s)
	}
}

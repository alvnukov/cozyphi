package project

import (
	"fmt"
	"strings"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
)

// WebQuarantine names the configured web.quarantine mode. It decodes as
// data — for observation and diagnostics — and authorizes nothing: the
// unchecked-delivery paths the modes once selected are gone, and the web
// tool fails closed until an explicit web model binding exists.
type WebQuarantine string

// Web quarantine modes.
const (
	// WebQuarantineReader is the default: read and find hand the bounded
	// fragment to a tool-less child model call, and only its answer reaches
	// the session.
	WebQuarantineReader WebQuarantine = "reader"
	// WebQuarantineOff used to hand the bounded fragment straight to the
	// session. It still decodes so existing configs load, with a load-time
	// warning that it no longer authorizes unchecked delivery.
	WebQuarantineOff WebQuarantine = "off"
)

// WebConfig is the resolved `web:` section. The library policy is the data
// cozy-tools consumes; Quarantine is cozyphi's own defense layer and has no
// meaning inside the library.
type WebConfig struct {
	Policy     cozyconfig.WebPolicy
	Quarantine WebQuarantine
	// Model is the web.model pin: the name of one entry in the configured
	// models list that the quarantined reader must run on. Empty means no
	// pin, which is one of the not-ready states — see WebBinding. It is a
	// reference, never a credential: resolving it against the model catalog
	// happens at admission, and nothing here authorizes a fallback to the
	// session model.
	Model string
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
	Model                *string     `yaml:"model"`
	Allow                *stringList `yaml:"allow"`
}

// defaultWebConfig is what a config file with no `web:` section runs on:
// network access off. The library's own default is on, but a capability that
// reaches the open internet is not something an existing config should gain by
// upgrading — and while protected web is not ready an enabled tool only
// refuses, so the opt-in is an explicit `enabled: true`, never a side effect
// of writing any other web key. The default flips to on once the protected
// web model binding lands.
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
	// Enabled stays off unless the file says `enabled: true` — see
	// defaultWebConfig for why no other key may switch the tool on.
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
		if mode == WebQuarantineOff {
			// The mode still decodes — it is data for observation — but it
			// no longer authorizes anything: unchecked delivery is gone.
			warnings = append(warnings,
				"web.quarantine: off no longer authorizes unchecked delivery: protected web research requires "+
					"an explicit web model binding, and the web tool refuses page content until one is configured "+
					"(see doc/web.md)")
		}
	}
	// The pin is kept verbatim (trimmed): whether it names a model is
	// decided at admission against the live catalog, not at load — a model
	// can appear after the file was written, and a load-time guess would
	// freeze the wrong verdict.
	if raw.Model != nil {
		w.Model = strings.TrimSpace(*raw.Model)
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

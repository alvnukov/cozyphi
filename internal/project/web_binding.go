package project

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// WebBindingState is the admission verdict for the web.model pin.
type WebBindingState int

const (
	// WebBindingUnset means no web.model pin is configured.
	WebBindingUnset WebBindingState = iota
	// WebBindingMissing means the pin names no model the session can reach.
	// There is no fallback: a stale pin is a broken configuration, not a
	// reason to borrow the session model.
	WebBindingMissing
	// WebBindingUnidentified means the model exists, but its actual recipient
	// has no stable non-secret account or connection identity. It must not be
	// treated as a resolved route because token replacement could redirect it.
	WebBindingUnidentified
	// WebBindingResolved means the pin names a configured model. Resolution
	// alone is not readiness: the consented capability preflight (spec D2)
	// has not landed yet, so the web tool still fails closed.
	WebBindingResolved
)

// WebBinding is the web.model pin resolved against the session's model
// catalog at admission time. It carries the full model config for the
// transport owner, but its display forms — Identity, Fingerprint,
// NotReadyReason — are built from non-secret fields only.
type WebBinding struct {
	pin   string
	model llm.ModelConfig
	state WebBindingState
}

// Binding resolves the web.model pin into an admission verdict. find maps a
// model name to its configured identity (session catalog, connected
// providers); a nil find resolves nothing, so a configured pin reports
// missing rather than guessing.
func (w WebConfig) Binding(find func(string) (llm.ModelConfig, bool)) WebBinding {
	pin := strings.TrimSpace(w.Model)
	if pin == "" {
		return WebBinding{state: WebBindingUnset}
	}
	b := WebBinding{pin: pin, state: WebBindingMissing}
	if find == nil {
		return b
	}
	if model, ok := find(pin); ok {
		b.model = model
		b.state = WebBindingUnidentified
		if strings.TrimSpace(model.ConnectionIdentity) != "" {
			b.state = WebBindingResolved
		}
	}
	return b
}

// State reports the admission verdict.
func (b WebBinding) State() WebBindingState { return b.state }

// Pin reports the configured web.model reference, resolved or not.
func (b WebBinding) Pin() string { return b.pin }

// Model returns the resolved connection config for the transport owner that
// will run the quarantined reader. ok is false unless the binding resolved.
// The returned value carries credentials: it must never be serialized into
// model-facing text, logs or diagnostics — use Identity or Fingerprint.
func (b WebBinding) Model() (model llm.ModelConfig, ok bool) {
	if b.state != WebBindingResolved {
		return llm.ModelConfig{}, false
	}
	return b.model, true
}

// Identity is the display form of the resolved route: provider, protocol,
// model name and endpoint. It contains no credentials.
func (b WebBinding) Identity() string {
	if b.state != WebBindingResolved && b.state != WebBindingUnidentified {
		return ""
	}
	parts := []string{
		fmt.Sprintf("provider=%q", b.model.ProviderID),
		fmt.Sprintf("protocol=%q", b.model.Protocol),
		fmt.Sprintf("model=%q", b.model.RequestModel()),
		fmt.Sprintf("endpoint=%q", safeEndpointIdentity(b.model.BaseURL)),
	}
	return strings.Join(parts, " ")
}

func safeEndpointIdentity(raw string) string {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "configured"
	}
	return (&url.URL{Scheme: endpoint.Scheme, Host: endpoint.Host}).String()
}

// Fingerprint is the invalidation key for anything checked against this
// route (the future capability preflight and its cached verdicts): it
// changes when the effective configuration — model, endpoint, protocol,
// effort or options — changes, so a changed route can never reuse a verdict
// recorded for the old one. Built from non-secret fields only.
func (b WebBinding) Fingerprint() string {
	if b.state != WebBindingResolved {
		return ""
	}
	return b.model.RequestBindingFingerprint()
}

// NotReadyReason is the actionable explanation the web tool reports while
// the binding keeps protected web from being ready. It names what is
// missing and never carries credentials.
func (b WebBinding) NotReadyReason() string {
	switch b.state {
	case WebBindingUnset:
		return "no web model binding is configured: set web.model: <name> in config.yaml to pin the model " +
			"the quarantined reader runs on (see doc/web.md)"
	case WebBindingMissing:
		return fmt.Sprintf(
			"web.model %q names no configured model: fix the pin to reference one entry of the models list — "+
				"there is no fallback to the session model", b.pin)
	case WebBindingUnidentified:
		return fmt.Sprintf(
			"web.model resolves to %s, but this route exposes no stable account or connection identity; "+
				"reconnect through a provider that supplies one — credentials cannot stand in for recipient identity",
			b.Identity())
	default:
		return fmt.Sprintf(
			"web.model resolves to %s, but the consented capability preflight that must verify this route "+
				"before first use has not landed yet, so protected web stays off (specs/protected-web-research.md D2)",
			b.Identity())
	}
}

package permission_test

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/permission"
)

// preflightRoute is the non-secret identity line of a resolved web binding,
// the way WebBinding.Identity() renders it.
const preflightRoute = "openai oauth · gpt-5.2 · api.openai.com"

func preflightRequest() permission.Request {
	return permission.Request{
		Action: permission.ActionWebPreflight,
		Tool:   "web",
		Target: preflightRoute,
	}
}

// TestWebPreflightConsentAsksByDefaultAndNamesTheRoute: the preflight spends
// quota on the configured web model route, so the consent question must name
// that route and ask by default — there is no knob that pre-approves it.
func TestWebPreflightConsentAsksByDefaultAndNamesTheRoute(t *testing.T) {
	gate := webGate(t, permission.DefaultPolicy())
	dec, reason := gate.Check(t.Context(), preflightRequest())
	if dec != permission.Ask {
		t.Fatalf("decision = %v, want Ask", dec)
	}
	if !strings.Contains(reason, "gpt-5.2") {
		t.Fatalf("reason does not name the route: %q", reason)
	}
}

// TestWebPreflightIsNotPreApprovedByWebHostAllow: consenting to fetch from a
// host is not consenting to spend the web model's quota (spec D8: the model
// provider and the target site are separate recipients). A catch-all
// permissions.web.allow entry must not silence the preflight question.
func TestWebPreflightIsNotPreApprovedByWebHostAllow(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WebAllow = []string{`.*`}
	gate := webGate(t, policy)

	dec, reason := gate.Check(t.Context(), preflightRequest())
	if dec != permission.Ask {
		t.Fatalf("web host allowlist pre-approved the preflight: %v", dec)
	}
	if strings.Contains(reason, "permissions.web.allow") {
		t.Fatalf("reason points at the host allowlist, which cannot apply: %q", reason)
	}
}

// TestWebPreflightDeniedWhenWebDisabled: web.enabled: false turns the whole
// web feature off, the model preflight included.
func TestWebPreflightDeniedWhenWebDisabled(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WebDisabled = true
	gate := webGate(t, policy)

	if dec, reason := gate.Check(t.Context(), preflightRequest()); dec != permission.Deny {
		t.Fatalf("decision = %v (%s), want Deny", dec, reason)
	}
}

// TestReadonlyModeFoldsWebPreflightAskToDeny: the preflight spends provider
// quota on outbound model requests — not a read action — so readonly folds
// the consent question into a refusal, same as any other non-read spend.
func TestReadonlyModeFoldsWebPreflightAskToDeny(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeReadonly
	gate := webGate(t, policy)

	if dec, _ := gate.Check(t.Context(), preflightRequest()); dec != permission.Deny {
		t.Fatalf("readonly decision = %v, want Deny", dec)
	}
}

// TestAutopilotFoldsWebPreflightAskToDeny: with no human to answer, the
// consent question must fail closed, not pass silently.
func TestAutopilotFoldsWebPreflightAskToDeny(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeAutopilot
	gate := webGate(t, policy)

	if dec, _ := gate.Check(t.Context(), preflightRequest()); dec != permission.Deny {
		t.Fatalf("autopilot decision = %v, want Deny", dec)
	}
}

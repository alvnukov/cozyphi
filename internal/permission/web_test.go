package permission_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/permission"
)

func webGate(t *testing.T, policy permission.Policy) *permission.StaticGate {
	t.Helper()
	gate, err := permission.NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatalf("new gate: %v", err)
	}
	return gate
}

func webRequest(t *testing.T, args map[string]any) permission.Request {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := permission.ExtractAt("web", raw, t.TempDir())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	return req
}

// TestExtractWebMapsTheSubjectOfEachAction: the target is what the user is
// shown, so it must be the thing the call actually reaches.
func TestExtractWebMapsTheSubjectOfEachAction(t *testing.T) {
	fetch := webRequest(t, map[string]any{"action": "fetch", "url": "https://Example.COM/docs?x=1"})
	if fetch.Action != permission.ActionWeb {
		t.Fatalf("action = %q, want web", fetch.Action)
	}
	if fetch.Target != "https://Example.COM/docs?x=1" {
		t.Fatalf("fetch target = %q, want the full URL", fetch.Target)
	}
	if fetch.Host != "example.com" {
		t.Fatalf("fetch host = %q, want the lowercased hostname", fetch.Host)
	}
	if fetch.Op != "fetch" {
		t.Fatalf("op = %q", fetch.Op)
	}

	search := webRequest(t, map[string]any{"action": "search", "query": "widget api", "provider": "google_cse"})
	if search.Target != "widget api" {
		t.Fatalf("search target = %q, want the query", search.Target)
	}
	if search.Host != "search:google_cse" {
		t.Fatalf("search host = %q", search.Host)
	}

	read := webRequest(t, map[string]any{"action": "read", "doc_id": "web_00112233445566aa", "raw": true})
	if read.Target != "web_00112233445566aa" {
		t.Fatalf("read target = %q, want the doc_id", read.Target)
	}
	if !read.Raw {
		t.Fatal("raw was not carried into the request")
	}
	if read.Host != "" {
		t.Fatalf("read host = %q, want empty: a cached read reaches no host", read.Host)
	}
}

// TestWebAsksByDefault: reaching the network is never silent.
func TestWebAsksByDefault(t *testing.T) {
	gate := webGate(t, permission.DefaultPolicy())
	dec, reason := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "fetch", "url": "https://example.com/docs"}))
	if dec != permission.Ask {
		t.Fatalf("decision = %v, want Ask", dec)
	}
	if !strings.Contains(reason, "https://example.com/docs") {
		t.Fatalf("reason does not name the URL: %q", reason)
	}
}

// TestWebAllowPreApprovesAHost mirrors MCPAllow: a host the user listed is
// not asked about again.
func TestWebAllowPreApprovesAHost(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WebAllow = []string{`^(.*\.)?golang\.org$`}
	gate := webGate(t, policy)

	if dec, _ := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "fetch", "url": "https://pkg.golang.org/x"})); dec != permission.Allow {
		t.Fatalf("allow-listed host was not allowed: %v", dec)
	}
	if dec, _ := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "fetch", "url": "https://evil.example/x"})); dec != permission.Ask {
		t.Fatalf("unlisted host = %v, want Ask", dec)
	}
}

// TestRawAlwaysAsks: the allow-list pre-approves reaching a host, never
// pouring its raw text into the model.
func TestRawAlwaysAsks(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WebAllow = []string{`.*`}
	gate := webGate(t, policy)

	dec, reason := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "read", "doc_id": "web_00112233445566aa", "raw": true}))
	if dec != permission.Ask {
		t.Fatalf("raw read under a wide allow-list = %v, want Ask", dec)
	}
	if !strings.Contains(reason, "raw page text") {
		t.Fatalf("reason does not say what raw means: %q", reason)
	}
}

// TestReadonlyModeStillAllowsWeb: fetching is a read of somebody else's
// document, not a mutation of this machine.
func TestReadonlyModeStillAllowsWeb(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeReadonly
	policy.WebAllow = []string{`^example\.com$`}
	gate := webGate(t, policy)

	if dec, _ := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "fetch", "url": "https://example.com/x"})); dec != permission.Allow {
		t.Fatalf("readonly denied an allow-listed fetch: %v", dec)
	}
	dec, _ := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "search", "query": "widget"}))
	if dec != permission.Ask {
		t.Fatalf("readonly folded a web Ask into Deny: %v", dec)
	}
}

// TestWebDisabledDenies: with web off the tool is absent, and any request
// that still arrives is refused rather than asked about.
func TestWebDisabledDenies(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.WebDisabled = true
	gate := webGate(t, policy)

	dec, reason := gate.Check(t.Context(), webRequest(t,
		map[string]any{"action": "fetch", "url": "https://example.com/x"}))
	if dec != permission.Deny {
		t.Fatalf("decision = %v, want Deny", dec)
	}
	if !strings.Contains(reason, "web.enabled") {
		t.Fatalf("reason does not point at the setting: %q", reason)
	}
}

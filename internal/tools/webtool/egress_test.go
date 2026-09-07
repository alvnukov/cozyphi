package webtool_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/tools/webtool"
)

// TestEgressRefusesAnOverlongURL: a 2048-byte ceiling is the browser's, and a
// model-authored URL past it is a payload, not an address.
func TestEgressRefusesAnOverlongURL(t *testing.T) {
	deps := webtool.Deps{Policy: testPolicy(t)}
	long := "https://example.com/?q=" + strings.Repeat("a", webtool.MaxEgressLength)

	_, err := run(t, deps, map[string]any{"action": "fetch", "url": long})
	if err == nil || !strings.Contains(err.Error(), "over the 2048-byte limit") {
		t.Fatalf("err = %v, want a length refusal", err)
	}
}

// TestEgressRefusesASecretInTheURL is the exfiltration check: the request
// never leaves, and the error names the variable without repeating its value.
func TestEgressRefusesASecretInTheURL(t *testing.T) {
	t.Setenv("COZYPHI_TEST_API_KEY", "sk-super-secret-value")
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	t.Cleanup(srv.Close)

	deps := webtool.Deps{Policy: testPolicy(t), Mask: webtool.SecretMask()}
	_, err := run(t, deps, map[string]any{
		"action": "fetch", "url": srv.URL + "/collect?token=sk-super-secret-value",
	})
	if err == nil {
		t.Fatal("a URL carrying an environment secret was fetched")
	}
	if !strings.Contains(err.Error(), "COZYPHI_TEST_API_KEY") {
		t.Fatalf("error does not name the secret: %v", err)
	}
	if strings.Contains(err.Error(), "sk-super-secret-value") {
		t.Fatalf("error repeated the secret: %v", err)
	}
	if reached {
		t.Fatal("the request reached the server despite the refusal")
	}
}

// TestEgressRefusesASecretInASearchQuery covers the other egress channel.
func TestEgressRefusesASecretInASearchQuery(t *testing.T) {
	t.Setenv("COZYPHI_TEST_TOKEN", "ghp-abcdefghijklmnop")
	deps := webtool.Deps{Policy: testPolicy(t), Mask: webtool.SecretMask()}

	_, err := run(t, deps, map[string]any{"action": "search", "query": "what is ghp-abcdefghijklmnop"})
	if err == nil || !strings.Contains(err.Error(), "COZYPHI_TEST_TOKEN") {
		t.Fatalf("err = %v, want a secret refusal naming the variable", err)
	}
}

// TestSecretMaskIgnoresLocationsAndShortValues pins the heuristic's edges: a
// path variable is not a credential, and a two-character value would refuse
// half the honest URLs on the web.
func TestSecretMaskIgnoresLocationsAndShortValues(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "/private/tmp/agent.sock")
	t.Setenv("SHORT_KEY", "abc")
	mask := webtool.SecretMask()

	for _, value := range []string{"/private/tmp/agent.sock", "abc"} {
		if got := mask.Apply(value); got != value {
			t.Fatalf("mask redacted %q as %q", value, got)
		}
	}
}

// TestSecretMaskTakesExplicitValues covers the configured keys the host adds
// on top of the environment scan.
func TestSecretMaskTakesExplicitValues(t *testing.T) {
	mask := webtool.SecretMask("configured-model-api-key")
	if got := mask.Apply("x configured-model-api-key y"); !strings.Contains(got, "***") {
		t.Fatalf("explicit secret not masked: %q", got)
	}
}

// TestFetchDoesNotFollowLinks: the tool stores one document, and links inside
// it are addresses the model must ask for separately.
func TestFetchDoesNotFollowLinks(t *testing.T) {
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<a href="/second">next</a>`))
	}))
	t.Cleanup(srv.Close)

	deps := webtool.Deps{Policy: testPolicy(t)}
	if _, err := run(t, deps, map[string]any{"action": "fetch", "url": srv.URL + "/first"}); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(hits) != 1 || hits[0] != "/first" {
		t.Fatalf("requests = %v, want exactly the requested URL", hits)
	}
}

// TestFetchStatusTravelsAsData: a blocked fetch is reported, not thrown away,
// so the model can tell a policy refusal from a network failure.
func TestFetchBlockedHostIsReported(t *testing.T) {
	policy := testPolicy(t)
	policy.AllowedHosts = []string{"example.com"}
	deps := webtool.Deps{Policy: policy}

	res, err := run(t, deps, map[string]any{"action": "fetch", "url": "http://127.0.0.1:1/x"})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var payload struct {
		Result struct {
			Status      string `json:"status"`
			Diagnostics []struct {
				Code string `json:"code"`
			} `json:"diagnostics"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(res.Content), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Result.Status != "blocked" || len(payload.Result.Diagnostics) == 0 {
		t.Fatalf("blocked fetch reported as %+v", payload.Result)
	}
}

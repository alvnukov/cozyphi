package agent

import (
	"strconv"
	"sync/atomic"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// bindingEngine builds an engine whose web section pins web.model and whose
// model catalog answers the pin. The session model endpoint counts calls:
// whatever the binding says, it must never be borrowed as the reader.
func bindingEngine(
	t *testing.T,
	pin string,
	catalog []llm.ModelConfig,
	modelHits **atomic.Int64,
) *Engine {
	t.Helper()
	on := true
	model, hits := countingServer(t, `{"choices":[]}`)
	*modelHits = hits
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "session-model", BaseURL: model.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       tools.DefaultTools(),
		AutoApprove: func() bool { return true },
		Web: WebOptionsFrom(project.WebConfig{
			Policy: cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()},
			Model:  pin,
		}, func(name string) (llm.ModelConfig, bool) {
			for _, m := range catalog {
				if m.Name == name {
					return m, true
				}
			}
			return llm.ModelConfig{}, false
		}),
	})
	require.NoError(t, err)
	return engine
}

// TestResolvedBindingStillRefusesNamingThePreflight: a pin that resolves
// against the catalog is the furthest readiness this ticket goes — the
// refusal names the missing capability preflight and the resolved identity,
// and costs the page server and the session model zero requests.
func TestResolvedBindingStillRefusesNamingThePreflight(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")
	reader := llm.ModelConfig{
		Name:               "reader-4o",
		ProviderID:         "openai",
		Protocol:           llm.ProtocolOpenAI,
		BaseURL:            "https://endpoint-user:userinfo-secret@gateway.example:8443/path-secret/v1?token=query-secret",
		APIKey:             "reader-secret",
		ConnectionIdentity: "account-fingerprint",
	}
	var modelHits *atomic.Int64
	engine := bindingEngine(t, "reader-4o", []llm.ModelConfig{reader}, &modelHits)

	var output, detail string
	msgs, active, _ := engine.roundSnapshot().executor.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "web", Arguments: `{"action":"fetch","url":` + strconv.Quote(page.URL) + `}`},
	}}, func(td session.ToolData) bool {
		if td.Run.Status == session.ToolDone {
			output, detail = td.Run.Output, td.Run.Detail
		}
		return true
	})
	require.True(t, active)
	require.Len(t, msgs, 1)
	msg := msgs[0]
	assert.Contains(t, msg.Content, "not ready")
	assert.Contains(t, msg.Content, "preflight",
		"a resolved binding fails closed on the missing preflight, not on the pin")
	assert.NotContains(t, msg.Content, "no web model binding",
		"the pin is configured; the refusal must not claim otherwise")
	for _, text := range []string{msg.Content, output, detail} {
		assert.Contains(t, text, "https://gateway.example:8443")
		for _, secret := range []string{
			"endpoint-user", "userinfo-secret", "path-secret", "query-secret", "reader-secret", "account-fingerprint",
		} {
			assert.NotContains(t, text, secret, "model-facing and transcript fields must not expose route credentials")
		}
	}
	assert.NotContains(t, msg.Content, "Widget API")
	assert.Zero(t, pageHits.Load(), "no acquisition while unready")
	assert.Zero(t, modelHits.Load(), "the session model is not the quarantine reader")
}

// TestStalePinRefusesWithoutSubstitution: web.model names a model the
// catalog no longer has — the configuration is broken, and the answer says
// so. The session model is running and reachable; it is still never
// consulted, because there is no fallback.
func TestStalePinRefusesWithoutSubstitution(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")
	var modelHits *atomic.Int64
	engine := bindingEngine(t, "removed-model", nil, &modelHits)

	msg := runWebCall(t, engine, `{"action":"fetch","url":`+strconv.Quote(page.URL)+`}`)
	assert.Contains(t, msg.Content, "not ready")
	assert.Contains(t, msg.Content, `"removed-model"`,
		"the refusal names the pin that no longer resolves")
	assert.Contains(t, msg.Content, "no fallback to the session model")
	assert.NotContains(t, msg.Content, "Widget API")
	assert.Zero(t, pageHits.Load())
	assert.Zero(t, modelHits.Load(),
		"a stale pin downgrades to not-ready, never to the session model")
}

// TestUnsetPinRefusesNamingTheSetting: no pin at all is the third distinct
// answer — it tells the user which setting to write, and mentions no model.
func TestUnsetPinRefusesNamingTheSetting(t *testing.T) {
	var modelHits *atomic.Int64
	engine := bindingEngine(t, "", nil, &modelHits)

	msg := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, msg.Content, "not ready")
	assert.Contains(t, msg.Content, "web.model",
		"the refusal names the setting that would pin the model")
	assert.Zero(t, modelHits.Load())
}

func TestWebBindingAdmissionTracksCatalogChanges(t *testing.T) {
	on := true
	var catalog []llm.ModelConfig
	model, modelHits := countingServer(t, `{"choices":[]}`)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "session-model", BaseURL: model.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       tools.DefaultTools(),
		AutoApprove: func() bool { return true },
		Web: WebOptionsFrom(project.WebConfig{
			Policy: cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()},
			Model:  "reader-4o",
		}, func(name string) (llm.ModelConfig, bool) {
			for _, candidate := range catalog {
				if candidate.Name == name {
					return candidate, true
				}
			}
			return llm.ModelConfig{}, false
		}),
	})
	require.NoError(t, err)

	missing := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, missing.Content, "names no configured model")

	catalog = []llm.ModelConfig{{
		Name: "reader-4o", ProviderID: "openai", BaseURL: "https://api.example.test/v1", APIKey: "secret",
	}}
	unidentified := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, unidentified.Content, "stable account or connection identity")

	catalog[0].ConnectionIdentity = "account-fingerprint"
	connected := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, connected.Content, "preflight")
	assert.Contains(t, connected.Content, `endpoint="https://api.example.test"`)

	catalog[0].BaseURL = "https://changed.example.test/other-route"
	routeChanged := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, routeChanged.Content, `endpoint="https://changed.example.test"`,
		"route changes must be resolved at admission rather than from the engine snapshot")

	catalog = nil
	removed := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, removed.Content, "names no configured model")
	assert.Zero(t, modelHits.Load(), "admission changes must not borrow the session model")
}

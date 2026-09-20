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
		Name:       "reader-4o",
		ProviderID: "openai",
		Protocol:   llm.ProtocolOpenAI,
		BaseURL:    "https://api.example.test/v1",
		APIKey:     "reader-secret",
	}
	var modelHits *atomic.Int64
	engine := bindingEngine(t, "reader-4o", []llm.ModelConfig{reader}, &modelHits)

	msg := runWebCall(t, engine, `{"action":"fetch","url":`+strconv.Quote(page.URL)+`}`)
	assert.Contains(t, msg.Content, "not ready")
	assert.Contains(t, msg.Content, "preflight",
		"a resolved binding fails closed on the missing preflight, not on the pin")
	assert.NotContains(t, msg.Content, "no web model binding",
		"the pin is configured; the refusal must not claim otherwise")
	assert.NotContains(t, msg.Content, "reader-secret",
		"the resolved route's credentials never reach model-facing text")
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

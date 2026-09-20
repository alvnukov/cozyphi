package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// sseText wraps one SSE content chunk carrying text.
func agentSSEText(text string) string {
	return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":%q}}]}\n\n", text)
}

// probeScript is the pinned web model route standing in for the provider: a
// counting SSE server that answers each preflight probe by recognizing its
// request body, exactly as the runner tests script it.
type probeScript struct {
	server *httptest.Server
	hits   *atomic.Int64
}

func newProbeScript(t *testing.T) *probeScript {
	t.Helper()
	hits := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		var reply string
		switch {
		case strings.Contains(string(body), "preflight-echo-7f3a"):
			reply = agentSSEText(`{"echo":"preflight-echo-7f3a"}`)
		case strings.Contains(string(body), `"bash"`):
			reply = fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\","+
				"\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":%q}}]}}]}\n\n",
				`{"command":"echo preflight"}`)
		default: // the vision probe carries image media
			reply = agentSSEText(`{"top":"red","bottom":"blue"}`)
		}
		_, _ = fmt.Fprint(w, reply)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return &probeScript{server: server, hits: hits}
}

// askRecorder records permission asks, counting the preflight consent
// question separately from the ordinary web tool-call asks, and answers
// everything with the configured verdict.
type askRecorder struct {
	approved      bool
	asks          atomic.Int64
	preflightAsks atomic.Int64
	lastPreflight atomic.Pointer[permission.Request]
}

func (a *askRecorder) askFunc() permission.AskFunc {
	return func(_ context.Context, req permission.Request, _ string) (permission.AskResult, error) {
		a.asks.Add(1)
		if req.Action == permission.ActionWebPreflight {
			a.preflightAsks.Add(1)
			a.lastPreflight.Store(&req)
		}
		return permission.AskResult{Approved: a.approved}, nil
	}
}

func approvingAsk() permission.AskFunc {
	return func(context.Context, permission.Request, string) (permission.AskResult, error) {
		return permission.AskResult{Approved: true}, nil
	}
}

func enabledPolicy(t *testing.T) cozyconfig.WebPolicy {
	t.Helper()
	on := true
	return cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()}
}

func readerModel(baseURL string) llm.ModelConfig {
	return llm.ModelConfig{
		Name:               "reader-4o",
		ProviderID:         "openai",
		Protocol:           llm.ProtocolOpenAI,
		BaseURL:            baseURL,
		APIKey:             "reader-secret",
		ConnectionIdentity: "account-fingerprint",
	}
}

// preflightEngine builds an engine whose web binding resolves to a probe
// script route and whose permission posture is a real default gate plus the
// given ask handler.
func preflightEngine(t *testing.T, ask permission.AskFunc, probeBase string) *Engine {
	t.Helper()
	// The web egress itself is pre-approved so the recorder's asks isolate
	// the preflight consent; web_preflight has no allowlist branch (D8), so
	// it still asks.
	policy := permission.DefaultPolicy()
	policy.WebAllow = []string{".*"}
	gate, err := permission.NewGate(policy, t.TempDir())
	require.NoError(t, err)
	session, _ := countingServer(t, `{"choices":[]}`)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "session-model", BaseURL: session.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       tools.DefaultTools(),
		Gate:        gate,
		Ask:         ask,
		Web: WebOptionsFrom(project.WebConfig{
			Policy: enabledPolicy(t),
			Model:  "reader-4o",
		}, func(string) (llm.ModelConfig, bool) {
			return readerModel(probeBase), true
		}),
	})
	require.NoError(t, err)
	return engine
}

func fetchCall(page *httptest.Server) string {
	return `{"action":"fetch","url":"` + page.URL + `"}`
}

// TestWebAdmissionNamesUndeliveredAdapter: with no shared account admission
// adapter (the production state until routing-openai-account-admission
// lands), the tool refusal names that task, spends zero requests and never
// raises the consent question — the order admission → consent is observable.
func TestWebAdmissionNamesUndeliveredAdapter(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")
	probe := newProbeScript(t)
	rec := &askRecorder{approved: true}
	engine := preflightEngine(t, rec.askFunc(), probe.server.URL)

	msg := runWebCall(t, engine, fetchCall(page))
	assert.Contains(t, msg.Content, "not ready")
	assert.Contains(t, msg.Content, "routing-openai-account-admission",
		"the refusal names the undelivered adapter instead of a generic preflight-missing line")
	assert.Zero(t, probe.hits.Load(), "no probe request without the admission adapter")
	assert.Zero(t, pageHits.Load(), "no acquisition while unready")
	assert.Zero(t, rec.preflightAsks.Load(), "consent is never raised when admission already said no")
}

// TestWebAdmissionAsksConsentOnceAndCachesVerdict: with a test admission
// hook, the preflight runs once per route: one consent question carrying the
// route identity and the web_preflight action, three probe requests, and the
// second call reuses the cached verdict without re-asking or re-probing.
func TestWebAdmissionAsksConsentOnceAndCachesVerdict(t *testing.T) {
	page, _ := countingServer(t, "<html><body>Widget API</body></html>")
	probe := newProbeScript(t)
	rec := &askRecorder{approved: true}
	engine := preflightEngine(t, rec.askFunc(), probe.server.URL)
	engine.web.admit = func(context.Context) error { return nil }

	first := runWebCall(t, engine, fetchCall(page))
	assert.NotContains(t, first.Content, "routing-openai-account-admission",
		"the adapter exists here; the boundary moved past it")
	assert.Equal(t, int64(1), rec.preflightAsks.Load(), "exactly one consent question")
	req := rec.lastPreflight.Load()
	require.NotNil(t, req)
	assert.Equal(t, permission.ActionWebPreflight, req.Action)
	assert.Equal(t, "web", req.Tool)
	assert.Contains(t, req.Target, "reader-4o", "the consent names the pinned route, not a placeholder")
	assert.Equal(t, int64(3), probe.hits.Load(), "one request per probe")

	second := runWebCall(t, engine, fetchCall(page))
	assert.NotContains(t, second.Content, "routing-openai-account-admission")
	assert.Equal(t, int64(3), probe.hits.Load(), "the cached verdict is reused, not re-probed")
	assert.Equal(t, int64(1), rec.preflightAsks.Load(), "consent is not re-asked for the same route")
}

// TestWebAdmissionConsentDeniedCachesRefusal: a denied consent fails closed,
// sends nothing, and is remembered for the route — one denial, not one per
// call. A restart or a config change re-asks; nothing else does.
func TestWebAdmissionConsentDeniedCachesRefusal(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")
	probe := newProbeScript(t)
	rec := &askRecorder{approved: false}
	engine := preflightEngine(t, rec.askFunc(), probe.server.URL)
	engine.web.admit = func(context.Context) error { return nil }

	msg := runWebCall(t, engine, fetchCall(page))
	assert.Contains(t, msg.Content, "consent")
	assert.Zero(t, probe.hits.Load(), "denied consent sends no probe")
	assert.Zero(t, pageHits.Load())

	runWebCall(t, engine, fetchCall(page))
	assert.Equal(t, int64(1), rec.preflightAsks.Load(), "the denial is cached for the route")
}

// TestWebAdmissionAskFailureIsNotCached: a broken ask channel is transport
// noise, not an answer — the call fails closed, but the route stays uncached
// so the next call can ask again once the channel works.
func TestWebAdmissionAskFailureIsNotCached(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")
	probe := newProbeScript(t)
	var calls atomic.Int64
	failingAsk := func(context.Context, permission.Request, string) (permission.AskResult, error) {
		calls.Add(1)
		return permission.AskResult{}, errors.New("ask channel closed")
	}
	engine := preflightEngine(t, failingAsk, probe.server.URL)
	engine.web.admit = func(context.Context) error { return nil }

	msg := runWebCall(t, engine, fetchCall(page))
	assert.Contains(t, msg.Content, "consent", "the call still fails closed")
	assert.Zero(t, probe.hits.Load(), "a failed ask sends no probe")
	assert.Zero(t, pageHits.Load())

	runWebCall(t, engine, fetchCall(page))
	assert.Equal(t, int64(2), calls.Load(), "the failed ask is not cached as a denial")
}

// TestWebAdmissionRerunsOnFingerprintChange: the verdict belongs to a route
// fingerprint, not to the engine — when the effective route changes, the
// preflight runs again against the new route.
func TestWebAdmissionRerunsOnFingerprintChange(t *testing.T) {
	page, _ := countingServer(t, "<html><body>Widget API</body></html>")
	probeA := newProbeScript(t)
	probeB := newProbeScript(t)

	base := atomic.Pointer[string]{}
	first := probeA.server.URL
	base.Store(&first)
	gate, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	session, _ := countingServer(t, `{"choices":[]}`)
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "session-model", BaseURL: session.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       tools.DefaultTools(),
		Gate:        gate,
		Ask:         approvingAsk(),
		Web: WebOptionsFrom(project.WebConfig{
			Policy: enabledPolicy(t),
			Model:  "reader-4o",
		}, func(string) (llm.ModelConfig, bool) {
			return readerModel(*base.Load()), true
		}),
	})
	require.NoError(t, err)
	engine.web.admit = func(context.Context) error { return nil }

	runWebCall(t, engine, fetchCall(page))
	assert.Equal(t, int64(3), probeA.hits.Load())

	next := probeB.server.URL
	base.Store(&next)
	runWebCall(t, engine, fetchCall(page))
	assert.Equal(t, int64(3), probeB.hits.Load(), "a changed fingerprint reruns the preflight")
	assert.Equal(t, int64(3), probeA.hits.Load(), "the old route is not probed again")
}

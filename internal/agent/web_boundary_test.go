package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// countingServer answers anything and counts how often it was reached. It
// plays both the web page and the model endpoint: the not-ready refusal must
// cost zero requests to either.
func countingServer(t *testing.T, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	hits := &atomic.Int64{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

// webPosture is one general permission posture the refusal matrix runs
// under. Each declares what its gate says about a fetch up front, so the
// not-ready refusal can be proven to be the tool's own in every posture.
type webPosture struct {
	name string
	// gate builds the engine's gate; a nil builder is no gate at all, which
	// the engine reads as allow-all.
	gate func(t *testing.T) permission.Gate
	// ask builds the ask handler answering the gate's asks, counting them;
	// a nil builder leaves every ask unanswered.
	ask func(asked *atomic.Int64) permission.AskFunc
	// wantGate is what the gate must say about the fetch before the tool
	// runs, and wantAsks how many asks the posture answers across the
	// matrix's calls.
	wantGate permission.Decision
	wantAsks int64
}

// webPostures are the general permission postures of the verification
// matrix: permissions switched off entirely, an observing gate whose every
// ask the user approves, and the session-wide allow-all bypass. None of
// them may release web content while protected web is not ready.
var webPostures = []webPosture{
	{
		name:     "off",
		wantGate: permission.Allow,
	},
	{
		name: "observe",
		gate: func(t *testing.T) permission.Gate {
			t.Helper()
			gate, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
			require.NoError(t, err)
			return gate
		},
		ask: func(asked *atomic.Int64) permission.AskFunc {
			return func(_ context.Context, _ permission.Request, _ string) (permission.AskResult, error) {
				asked.Add(1)
				return permission.AskResult{Approved: true}, nil
			}
		},
		wantGate: permission.Ask,
		wantAsks: 4, // fetch, two reads and a search each ask once
	},
	{
		name: "allow-all",
		gate: func(t *testing.T) permission.Gate {
			t.Helper()
			inner, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
			require.NoError(t, err)
			enabled := &atomic.Bool{}
			enabled.Store(true)
			return &permission.BypassGate{Inner: inner, Enabled: enabled}
		},
		wantGate: permission.Allow,
	},
}

// boundaryEngine builds an engine with web enabled from the given config
// section under the given general permission posture (nil gate and ask mean
// the engine's allow-all default), and a model endpoint that counts any call
// the legacy session-model reader would have made.
func boundaryEngine(
	t *testing.T,
	cfg project.WebConfig,
	gate permission.Gate,
	ask permission.AskFunc,
	modelHits **atomic.Int64,
) *Engine {
	t.Helper()
	model, hits := countingServer(t, `{"choices":[]}`)
	*modelHits = hits
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: model.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		// An explicit tool set keeps the plan gate out of the way: this test
		// exercises the web boundary, not plan enforcement.
		Tools:       tools.DefaultTools(),
		Gate:        gate,
		Ask:         ask,
		AutoApprove: func() bool { return true },
		Web:         WebOptionsFrom(cfg, nil),
	})
	require.NoError(t, err)
	return engine
}

func runWebCall(t *testing.T, engine *Engine, arguments string) llm.Message {
	t.Helper()
	msgs, active, _ := engine.roundSnapshot().executor.run(t.Context(), []llm.ToolCall{{
		ID:       "c1",
		Function: llm.Function{Name: "web", Arguments: arguments},
	}}, func(session.ToolData) bool { return true })
	require.True(t, active)
	require.Len(t, msgs, 1)
	return msgs[0]
}

// TestEnabledWebRefusesEveryPathWhileUnready is the engine-level refusal
// matrix: enabled web, a legacy quarantine:off section, a raw request and
// every general permission posture — off, observe, allow-all — all meet the
// same not-ready refusal, and neither the page server nor the model endpoint
// saw a request.
func TestEnabledWebRefusesEveryPathWhileUnready(t *testing.T) {
	page, pageHits := countingServer(t, "<html><body>Widget API</body></html>")

	for _, posture := range webPostures {
		for _, quarantine := range []project.WebQuarantine{project.WebQuarantineReader, project.WebQuarantineOff} {
			t.Run(posture.name+"/quarantine="+string(quarantine), func(t *testing.T) {
				on := true
				cfg := project.WebConfig{
					Policy: cozyconfig.WebPolicy{
						Enabled:      &on,
						CacheDir:     t.TempDir(),
						AllowedHosts: []string{"127.0.0.1"},
					},
					Quarantine: quarantine,
				}
				var modelHits *atomic.Int64
				asked := &atomic.Int64{}
				var gate permission.Gate
				if posture.gate != nil {
					gate = posture.gate(t)
				}
				var ask permission.AskFunc
				if posture.ask != nil {
					ask = posture.ask(asked)
				}
				engine := boundaryEngine(t, cfg, gate, ask, &modelHits)
				require.True(t, engine.HasTool("web"), "enabled web still carries the tool; it is the run that refuses")

				// The posture's gate speaks first — off and allow-all permit
				// the fetch, observe only asks — so the refusal below is the
				// tool's own, never the gate's.
				dec, _ := engine.executor.gate.Check(t.Context(), permission.Request{
					Action: permission.ActionWeb, Op: "fetch", Host: "127.0.0.1", Target: page.URL,
				})
				require.Equal(t, posture.wantGate, dec, "the gate must hold its posture; the tool refuses anyway")

				calls := []string{
					`{"action":"fetch","url":` + strconv.Quote(page.URL) + `}`,
					`{"action":"read","doc_id":"web_0000","question":"q"}`,
					`{"action":"read","doc_id":"web_0000","raw":true}`,
					`{"action":"search","query":"widget api"}`,
				}
				for _, call := range calls {
					msg := runWebCall(t, engine, call)
					assert.Contains(t, msg.Content, "not ready", call)
					assert.Contains(t, msg.Content, "web model binding", call)
					assert.NotContains(t, msg.Content, "Widget API", call)
				}
				assert.Equal(t, posture.wantAsks, asked.Load(),
					"every ask the posture answers is an observed approval, not a release")
				assert.Zero(t, pageHits.Load(), "no acquisition while unready")
				assert.Zero(t, modelHits.Load(), "the session model is not the quarantine reader")
				assert.False(t, engine.turnWeb.Tainted(), "a refusal delivers no web text, so it must not taint")
			})
		}
	}
}

// TestEnabledTrueIsNoReadinessClaim: web.enabled: true used to be the whole
// opt-in. The tool still registers — but webFacts and the run both refuse to
// call that protected readiness.
func TestEnabledTrueIsNoReadinessClaim(t *testing.T) {
	on := true
	cfg := project.WebConfig{Policy: cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()}}
	var modelHits *atomic.Int64
	engine := boundaryEngine(t, cfg, nil, nil, &modelHits)

	msg := runWebCall(t, engine, `{"action":"fetch","url":"https://example.com"}`)
	assert.Contains(t, msg.Content, "not ready",
		"web.enabled: true alone must not deliver anything")
}

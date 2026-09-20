package webpreflight_test

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

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/webpreflight"
)

// probeServer counts requests and answers from scripted replies picked by a
// caller-supplied matcher on the request body. It stands in for the pinned
// web model route; no request ever leaves the test.
type probeServer struct {
	requests atomic.Int32
	server   *httptest.Server
}

func newProbeServer(pick func(body string) string, fail func(body string) (int, string)) *probeServer {
	ps := &probeServer{}
	ps.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		if fail != nil {
			if status, msg := fail(string(body)); status != 0 {
				w.WriteHeader(status)
				_, _ = fmt.Fprint(w, msg)
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, pick(string(body)))
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	return ps
}

func (ps *probeServer) URL() string { return ps.server.URL }

func (ps *probeServer) Count() int { return int(ps.requests.Load()) }

func routeConfig(baseURL string) llm.ModelConfig {
	return llm.ModelConfig{
		Name:     "web-model",
		APIName:  "web-model",
		Protocol: llm.ProtocolOpenAI,
		BaseURL:  baseURL,
		APIKey:   "test-key",
	}
}

// sseText wraps one SSE content chunk carrying text.
func sseText(text string) string {
	return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":%q}}]}\n\n", text)
}

// TestRunWithoutAdmissionIsUnavailableAndSpendsNothing: with no shared
// account admission adapter delivered, the preflight must report unavailable
// naming the owning task, make zero requests and never raise the consent
// question — consent spam for a route that cannot run is worse than silence.
func TestRunWithoutAdmissionIsUnavailableAndSpendsNothing(t *testing.T) {
	ps := newProbeServer(
		func(string) string { t.Error("no request may happen without admission"); return "" },
		nil,
	)
	consented := atomic.Bool{}
	runner := webpreflight.New(routeConfig(ps.URL()), "openai oauth · web-model · api.example", nil,
		nil, // admission adapter not delivered
		func(context.Context, string, int) error {
			consented.Store(true)
			return nil
		})

	verdict, err := runner.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, webpreflight.StatusUnavailable, verdict.Status)
	require.Contains(t, verdict.Blocker, "routing-openai-account-admission")
	require.Zero(t, ps.Count(), "requests sent without admission")
	require.Zero(t, verdict.Requests)
	require.False(t, consented.Load(), "consent question raised without admission")
}

// TestRunDeniedConsentSendsNothing: a denied (or failed) consent is a
// structured refusal, not a verdict — and not a single request leaves.
func TestRunDeniedConsentSendsNothing(t *testing.T) {
	ps := newProbeServer(
		func(string) string { t.Error("no request may happen after denied consent"); return "" },
		nil,
	)
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", nil,
		func(context.Context) error { return nil },
		func(context.Context, string, int) error { return errors.New("denied by user") })

	verdict, err := runner.Run(t.Context())
	require.ErrorContains(t, err, "denied by user")
	require.ErrorContains(t, err, "consent")
	require.Zero(t, ps.Count(), "requests sent after denied consent")
	require.Zero(t, verdict.Requests)
}

// sseToolCall wraps one SSE chunk: the scripted route answering the tools
// probe with a bash tool call, exactly the decoy shape the runner offers.
func sseToolCall() string {
	return "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"command\\\":\\\"echo preflight\\\"}\"}}]}}]}\n\n"
}

func bashDecoy() []llm.ToolDefinition {
	return []llm.ToolDefinition{{
		Name:        "bash",
		Description: "Run a shell command.",
	}}
}

func allowAll() (webpreflight.AdmissionFunc, webpreflight.ConsentFunc) {
	return func(context.Context) error { return nil },
		func(context.Context, string, int) error { return nil }
}

// scriptedRoute answers each probe by recognizing its request body: the
// structured probe carries its token, the tool probe offers the bash decoy,
// the vision probe carries the image media.
func scriptedRoute(structured, tools, vision string) func(string) string {
	return func(body string) string {
		switch {
		case strings.Contains(body, "preflight-echo-7f3a"):
			return structured
		case strings.Contains(body, `"bash"`):
			return tools
		default:
			return vision
		}
	}
}

// TestRunReadyWhenAllProbesPass: three passing probes, three requests, one
// per probe — the budget the consent question stated.
func TestRunReadyWhenAllProbesPass(t *testing.T) {
	ps := newProbeServer(scriptedRoute(
		sseText(`{"echo":"preflight-echo-7f3a"}`),
		sseToolCall(),
		sseText(`{"top":"red","bottom":"blue"}`),
	), nil)
	admit, consent := allowAll()
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", bashDecoy(), admit, consent)

	verdict, err := runner.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, webpreflight.StatusReady, verdict.Status)
	require.Equal(t, 3, verdict.Requests)
	require.Equal(t, 3, ps.Count())
	require.True(t, verdict.Structured.Pass)
	require.True(t, verdict.Tools.Pass)
	require.True(t, verdict.Vision.Pass)
	require.Empty(t, verdict.Blocker)
}

// TestRunLimitedWhenRouteRejectsImages: a route that fails the image request
// keeps its text pipeline — limited, with image reads declared unavailable,
// never silently degraded to OCR-only (D2).
func TestRunLimitedWhenRouteRejectsImages(t *testing.T) {
	ps := newProbeServer(scriptedRoute(
		sseText(`{"echo":"preflight-echo-7f3a"}`),
		sseToolCall(),
		sseText(`{"top":"red","bottom":"blue"}`),
	), func(body string) (int, string) {
		// Only the vision probe carries image media; reject exactly that request.
		if strings.Contains(body, "image") {
			return http.StatusBadRequest, `{"error":{"message":"image input is not supported"}}`
		}
		return 0, ""
	})
	admit, consent := allowAll()
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", bashDecoy(), admit, consent)

	verdict, err := runner.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, webpreflight.StatusLimited, verdict.Status)
	require.False(t, verdict.Vision.Pass)
	require.True(t, verdict.Structured.Pass)
	require.True(t, verdict.Tools.Pass)
	require.Contains(t, verdict.Blocker, "image reads stay unavailable")
	require.Equal(t, 3, verdict.Requests)
}

// TestRunNotReadyWhenStructuredReplyIsProse: the strict parse is load
// bearing — prose around the JSON is a fail, not a pass.
func TestRunNotReadyWhenStructuredReplyIsProse(t *testing.T) {
	ps := newProbeServer(scriptedRoute(
		sseText("Sure! Here is your object: {\"echo\":\"preflight-echo-7f3a\"} hope that helps"),
		sseToolCall(),
		sseText(`{"top":"red","bottom":"blue"}`),
	), nil)
	admit, consent := allowAll()
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", bashDecoy(), admit, consent)

	verdict, err := runner.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, webpreflight.StatusNotReady, verdict.Status)
	require.False(t, verdict.Structured.Pass)
	require.Contains(t, verdict.Blocker, "structured replies failed")
}

// TestRunNotReadyWhenModelAnswersProseInsteadOfCalling: a route that will
// not issue a tool call cannot host the quarantine reader's decoy offers.
func TestRunNotReadyWhenModelAnswersProseInsteadOfCalling(t *testing.T) {
	ps := newProbeServer(scriptedRoute(
		sseText(`{"echo":"preflight-echo-7f3a"}`),
		sseText("I would rather just tell you the output: preflight"),
		sseText(`{"top":"red","bottom":"blue"}`),
	), nil)
	admit, consent := allowAll()
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", bashDecoy(), admit, consent)

	verdict, err := runner.Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, webpreflight.StatusNotReady, verdict.Status)
	require.False(t, verdict.Tools.Pass)
	require.Contains(t, verdict.Blocker, "tool-call round failed")
}

// TestRunHonorsCancellationBetweenProbes: a cancelled run stops — no fourth
// request, no verdict dressed up as fact.
func TestRunHonorsCancellationBetweenProbes(t *testing.T) {
	ps := newProbeServer(scriptedRoute(
		sseText(`{"echo":"preflight-echo-7f3a"}`),
		sseToolCall(),
		sseText(`{"top":"red","bottom":"blue"}`),
	), nil)
	admit, consent := allowAll()
	runner := webpreflight.New(routeConfig(ps.URL()), "identity", bashDecoy(), admit, consent)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := runner.Run(ctx)
	require.ErrorContains(t, err, "cancelled")
}

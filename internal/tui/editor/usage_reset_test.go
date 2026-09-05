package editor

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestUsageConfirmedResetRefreshesThroughEditor(t *testing.T) {
	var resets, reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			reads.Add(1)
			_, _ = fmt.Fprintf(
				w,
				`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":50,"limit_window_seconds":18000}},"rate_limit_reset_credits":{"available_count":%d}}`,
				2-resets.Load(),
			)
		case "/backend-api/wham/profiles/me":
			_, _ = w.Write(
				[]byte(
					`{"stats":{"lifetime_tokens":987654,"daily_usage_buckets":[{"start_date":"2026-09-01","tokens":987654}]}}`,
				),
			)
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			if r.Method != http.MethodPost {
				t.Error("reset must POST")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			resets.Add(1)
			_, _ = w.Write([]byte(`{"code":"reset","windows_reset":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	e := newQuotaResetEditor(t, server.URL)
	e.ShowUsage()
	waitForUsageText(t, e, "2 available")
	text := usageText(e)
	require.Contains(t, text, "Session")
	require.NotContains(t, text, "lifetime")
	require.NotContains(t, text, "daily")
	require.NotContains(t, text, "987654")
	usageKey(e, 'x')
	require.Zero(t, resets.Load(), "arming does not spend")
	usageKey(e, 'n')
	require.Zero(t, resets.Load(), "cancel does not spend")
	usageKey(e, 'x')
	require.Contains(t, usageText(e), "Spend one reset credit")
	usageKey(e, 'y')
	usageKey(e, 'y')
	waitForUsageText(t, e, "1 available")
	require.Contains(
		t,
		usageText(e),
		"[Reset limit x]",
		"fresh quotas restore the action without another manual refresh",
	)
	require.Equal(t, int32(1), resets.Load(), "duplicate key presses must not repeat the mutation")
	require.GreaterOrEqual(t, reads.Load(), int32(2), "completion fetches fresh quotas")
}

func TestUsageResetSuccessSurvivesRefreshFailure(t *testing.T) {
	var resets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			if resets.Load() > 0 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"rate_limit_reset_credits":{"available_count":1}}`))
		case "/backend-api/wham/profiles/me":
			_, _ = w.Write([]byte(`{}`))
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			resets.Add(1)
			_, _ = w.Write([]byte(`{"code":"reset","windows_reset":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	e := newQuotaResetEditor(t, server.URL)
	e.ShowUsage()
	waitForUsageText(t, e, "1 available")
	usageKey(e, 'x')
	require.Contains(t, usageText(e), "Spend one reset credit")
	usageKey(e, 'y')
	waitForUsageText(t, e, "503")
	require.Contains(
		t,
		strings.ToLower(usageText(e)),
		"reset",
		"mutation result remains visible separately from read error",
	)
	require.Equal(t, int32(1), resets.Load())
}

func newQuotaResetEditor(t *testing.T, origin string) *Editor {
	t.Helper()
	home, cwd := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	t.Setenv("COZYPHI_BASE_URL", "")
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	local, err := url.Parse(origin)
	require.NoError(t, err)
	client := util.DefaultHTTPClient()
	previous := client.Transport
	transport := util.SharedHTTPTransport().Clone()
	transport.Proxy = nil
	// Every outgoing request is redirected to this synthetic server or refused;
	// no production network target can be reached, including on a test failure.
	client.Transport = usageLocalTransport{local: local, base: transport}
	t.Cleanup(func() { client.Transport = previous; transport.CloseIdleConnections() })
	credentials := strings.Replace(
		subscriptionCredentials,
		`"type": "oauth",`,
		`"type": "oauth", "account_id": "synthetic-account",`,
		1,
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(proj.Global().CredentialsFile()), 0o755))
	require.NoError(t, os.WriteFile(proj.Global().CredentialsFile(), []byte(credentials), 0o600))
	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)
	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "", "", 0, nil, nil)
	require.NoError(t, e.SetModel("openai/gpt-5.5"))
	return e
}

func usageKey(e *Editor, r rune) {
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
}

func usageText(e *Editor) string {
	return components.SurfaceText(e.usagepane.Draw(components.DrawContext{
		Max: components.Size{Width: 120, Height: 35}, Method: xui.WidthUnicode,
	}))
}

func waitForUsageText(t *testing.T, e *Editor, text string) {
	t.Helper()
	require.Eventually(t, func() bool {
		e.drainBus()
		return strings.Contains(usageText(e), text)
	}, 5*time.Second, 5*time.Millisecond, "usage did not reach expected state")
}

type usageLocalTransport struct {
	local *url.URL
	base  http.RoundTripper
}

func (tr usageLocalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != "chatgpt.com" || !strings.HasPrefix(req.URL.Path, "/backend-api/wham/") {
		return nil, errors.New("test refuses non-quota network requests")
	}
	local := req.Clone(req.Context())
	local.URL.Scheme, local.URL.Host = tr.local.Scheme, tr.local.Host
	local.Host = tr.local.Host
	return tr.base.RoundTrip(local)
}

package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

const bootSentinel = "COMPACT-BOOT-SENTINEL"

// bootHookedEngine builds an engine whose session_start hook counts its runs
// and answers every compact refire with reply.
func bootHookedEngine(t *testing.T, url string, lifecycle bool, parentID, reply string) (*Engine, *atomic.Int32) {
	t.Helper()
	engine, err := NewEngine(EngineOpts{
		Model:          llm.ModelConfig{Name: "fake", BaseURL: url, APIKey: "x", ContextWindow: 100000},
		SessionOpts:    SessionOpts{Cwd: t.TempDir(), ParentID: parentID},
		LifecycleHooks: lifecycle,
	})
	require.NoError(t, err)
	var calls atomic.Int32
	engine.SetHooks(hooks.NewManager(hooks.Entry{
		Kind: hooks.KindSessionStart,
		Hook: hooks.FuncHook{
			HookName: "boot",
			Sess: func(_ context.Context, ev hooks.SessionEvent) (hooks.SessionResult, error) {
				calls.Add(1)
				// The manager runs hooks on their own goroutines: assert, not require.
				assert.Equal(t, hooks.ReasonCompact, ev.Reason)
				assert.Equal(t, engine.SessionID(), ev.SessionID)
				assert.Equal(t, engine.SessionCwd(), ev.Cwd)
				return hooks.SessionResult{Context: reply}, nil
			},
		},
	}))
	return engine, &calls
}

func compactingEngine(t *testing.T, lifecycle bool, parentID string) (*Engine, *atomic.Int32, func() []string) {
	t.Helper()
	server, _, bodies := fakeContextServer(t, "SUMMARY", func(int32) string { return sseTextChunk() })
	engine, calls := bootHookedEngine(t, server.URL, lifecycle, parentID, bootSentinel)
	seedTwoTurnHistory(t, engine)
	return engine, calls, bodies
}

func TestCompactionRefiresSessionStartAndDeliversOnce(t *testing.T) {
	engine, calls, bodies := compactingEngine(t, true, "")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.EqualValues(t, 1, calls.Load())

	drainLoop(t, engine, "after compact")
	drainLoop(t, engine, "and again")
	all := bodies()
	require.Equal(t, 1, strings.Count(all[len(all)-2], bootSentinel))
	require.Equal(t, 1, strings.Count(all[len(all)-1], bootSentinel))
}

func TestOverflowCompactionRefiresSessionStart(t *testing.T) {
	server, _, bodies := overflowContextServer(t, "SUMMARY-OF-OVERFLOW-HISTORY")
	engine, calls := bootHookedEngine(t, server.URL, true, "", bootSentinel)
	seedLargeHistory(t, engine, 40)

	drainLoop(t, engine, "overflowing turn")
	require.EqualValues(t, 1, calls.Load(), "overflow recovery is a compaction too")

	drainLoop(t, engine, "next turn")
	all := bodies()
	require.Equal(t, 1, strings.Count(all[len(all)-1], bootSentinel))
}

func TestUnsuccessfulCompactionRunsNoHook(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"message":"summary refused"}}`)
	}))
	t.Cleanup(failing.Close)

	t.Run("summary request fails", func(t *testing.T) {
		engine, calls := bootHookedEngine(t, failing.URL, true, "", bootSentinel)
		seedTwoTurnHistory(t, engine)
		require.Error(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
		require.Zero(t, calls.Load())
	})

	t.Run("context cancelled", func(t *testing.T) {
		engine, calls, _ := compactingEngine(t, true, "")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.Error(t, engine.CompactNow(ctx, func(session.Event) bool { return true }))
		require.Zero(t, calls.Load())
	})

	t.Run("consumer stops before the summary", func(t *testing.T) {
		engine, calls, _ := compactingEngine(t, true, "")
		require.ErrorIs(t, engine.CompactNow(t.Context(), func(session.Event) bool { return false }), context.Canceled)
		require.Zero(t, calls.Load())
	})
}

func TestCompactionWithoutLifecycleRunsNoHook(t *testing.T) {
	engine, calls, _ := compactingEngine(t, false, "")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.Zero(t, calls.Load())
}

func TestChildCompactionRunsNoHook(t *testing.T) {
	engine, calls, _ := compactingEngine(t, true, "parent-session")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.Zero(t, calls.Load())
}

func TestEmptyRefireKeepsPendingContext(t *testing.T) {
	server, _, bodies := fakeContextServer(t, "SUMMARY", func(int32) string { return sseTextChunk() })
	engine, calls := bootHookedEngine(t, server.URL, true, "", "")
	seedTwoTurnHistory(t, engine)

	engine.QueueSessionContext("RESUME-SENTINEL")
	require.NoError(t, engine.CompactNow(t.Context(), func(session.Event) bool { return true }))
	require.EqualValues(t, 1, calls.Load())

	drainLoop(t, engine, "after compact")
	all := bodies()
	require.Equal(t, 1, strings.Count(all[len(all)-1], "RESUME-SENTINEL"),
		"a refire with nothing to add leaves the pending bootstrap alone")
}

// queuedSessionContext peeks at the parked reminder without draining it.
func queuedSessionContext(engine *Engine) string {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.sessionContext
}

func TestTrimRefiresSessionStart(t *testing.T) {
	engine, calls, bodies := compactingEngine(t, true, "")
	keep := engine.ContextReport().Items[2].EntryID

	require.NoError(t, engine.TrimContextFrom(t.Context(), keep))
	// The refire runs in the background: trimming is a UI action.
	require.Eventually(t, func() bool { return queuedSessionContext(engine) != "" }, 5*time.Second, 5*time.Millisecond)
	require.EqualValues(t, 1, calls.Load())

	drainLoop(t, engine, "after trim")
	all := bodies()
	require.Equal(t, 1, strings.Count(all[len(all)-1], bootSentinel))
}

func TestFailedTrimRunsNoHook(t *testing.T) {
	engine, calls, _ := compactingEngine(t, true, "")
	require.Error(t, engine.TrimContextFrom(t.Context(), "no-such-entry"))
	require.Never(t, func() bool { return calls.Load() > 0 }, 100*time.Millisecond, 5*time.Millisecond)
}

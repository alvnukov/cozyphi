package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerNilNoop(t *testing.T) {
	var m *Manager
	ev := Event{Tool: "bash", Input: json.RawMessage(`{"command":"ls"}`)}
	pre := m.PreTool(context.Background(), ev)
	assert.False(t, pre.Denied)
	assert.JSONEq(t, `{"command":"ls"}`, string(pre.Input))
	assert.Empty(t, pre.Context)

	post := m.PostTool(context.Background(), ev)
	assert.Empty(t, post.Context)
	assert.False(t, post.Stop)
}

func TestManagerPreDenyShortCircuit(t *testing.T) {
	var secondCalled atomic.Bool
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "deny",
			MatchFn:  MatchTool("bash"),
			Pre: func(_ context.Context, _ Event) (PreResult, error) {
				return PreResult{Action: ActionDeny, Reason: "nope"}, nil
			},
		}, Kind: KindPreTool},
		Entry{Hook: FuncHook{
			HookName: "second",
			Pre: func(_ context.Context, _ Event) (PreResult, error) {
				secondCalled.Store(true)
				return PreResult{Action: ActionAllow}, nil
			},
		}, Kind: KindPreTool},
	)

	out := m.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, out.Denied)
	assert.Equal(t, "nope", out.Reason)
	assert.False(t, secondCalled.Load(), "later pre hooks must not run after Deny")
}

func TestManagerPreMatchSkips(t *testing.T) {
	var called atomic.Bool
	m := NewManager(Entry{Hook: FuncHook{
		HookName: "bash-only",
		MatchFn:  MatchTool("bash"),
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			called.Store(true)
			return PreResult{Action: ActionDeny, Reason: "x"}, nil
		},
	}, Kind: KindPreTool})

	out := m.PreTool(context.Background(), Event{Tool: "write", Input: json.RawMessage(`{}`)})
	assert.False(t, out.Denied)
	assert.False(t, called.Load())
}

func TestManagerPreChainedModify(t *testing.T) {
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "add-timeout",
			Pre: func(_ context.Context, ev Event) (PreResult, error) {
				var v map[string]any
				require.NoError(t, json.Unmarshal(ev.Input, &v))
				v["timeout"] = 30
				b, _ := json.Marshal(v)
				return PreResult{Action: ActionModify, Input: b}, nil
			},
		}, Kind: KindPreTool},
		Entry{Hook: FuncHook{
			HookName: "prefix-cmd",
			Pre: func(_ context.Context, ev Event) (PreResult, error) {
				var v map[string]any
				require.NoError(t, json.Unmarshal(ev.Input, &v))
				v["command"] = "safe " + v["command"].(string)
				b, _ := json.Marshal(v)
				return PreResult{Action: ActionModify, Input: b, Context: "rewrote"}, nil
			},
		}, Kind: KindPreTool},
	)

	out := m.PreTool(context.Background(), Event{
		Tool:  "bash",
		Input: json.RawMessage(`{"command":"rm -rf /tmp/x"}`),
	})
	require.False(t, out.Denied)
	assert.JSONEq(t, `{"command":"safe rm -rf /tmp/x","timeout":30}`, string(out.Input))
	assert.Equal(t, "rewrote", out.Context)
}

func TestManagerPreErrorFailOpenAndFailClosed(t *testing.T) {
	boom := errors.New("boom")

	open := NewManager(Entry{Hook: FuncHook{
		HookName: "flaky",
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			return PreResult{}, boom
		},
	}, Kind: KindPreTool})
	out := open.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{"a":1}`)})
	assert.False(t, out.Denied)
	assert.JSONEq(t, `{"a":1}`, string(out.Input))

	closed := NewManager(Entry{Hook: FuncHook{
		HookName: "strict",
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			return PreResult{}, boom
		},
	}, Kind: KindPreTool, FailClosed: true})
	out = closed.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, out.Denied)
	assert.Contains(t, out.Reason, "fail_closed")
	assert.Contains(t, out.Reason, "boom")
}

func TestManagerPreModifyEmptyFailOpen(t *testing.T) {
	m := NewManager(Entry{Hook: FuncHook{
		HookName: "bad-modify",
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			return PreResult{Action: ActionModify}, nil
		},
	}, Kind: KindPreTool})
	in := json.RawMessage(`{"command":"ls"}`)
	out := m.PreTool(context.Background(), Event{Tool: "bash", Input: in})
	assert.False(t, out.Denied)
	assert.JSONEq(t, `{"command":"ls"}`, string(out.Input))
}

func TestManagerPostContextAggregateAndTruncate(t *testing.T) {
	big := strings.Repeat("x", MaxContextBytes)
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "a",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				return PostResult{Context: "one"}, nil
			},
		}, Kind: KindPostTool},
		Entry{Hook: FuncHook{
			HookName: "b",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				return PostResult{Context: big}, nil
			},
		}, Kind: KindPostTool},
	)

	out := m.PostTool(context.Background(), Event{Tool: "bash", Output: "ok"})
	assert.False(t, out.Stop)
	assert.LessOrEqual(t, len(out.Context), MaxContextBytes)
	assert.Contains(t, out.Context, "one")
}

func TestManagerPostStopAndFailClosed(t *testing.T) {
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "stopper",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				return PostResult{Stop: true, Reason: "limit"}, nil
			},
		}, Kind: KindPostTool},
		Entry{Hook: FuncHook{
			HookName: "err",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				return PostResult{}, errors.New("disk")
			},
		}, Kind: KindPostTool, FailClosed: true},
	)
	out := m.PostTool(context.Background(), Event{Tool: "write"})
	assert.True(t, out.Stop)
	assert.Contains(t, out.Reason, "limit")
	assert.Contains(t, out.Reason, "fail_closed")
}

func TestManagerPostErrorFailOpen(t *testing.T) {
	m := NewManager(Entry{Hook: FuncHook{
		HookName: "flaky",
		Post: func(_ context.Context, _ Event) (PostResult, error) {
			return PostResult{}, errors.New("nope")
		},
	}, Kind: KindPostTool})
	out := m.PostTool(context.Background(), Event{Tool: "bash"})
	assert.False(t, out.Stop)
	assert.Empty(t, out.Context)
}

func TestManagerPostAsyncDetached(t *testing.T) {
	done := make(chan struct{})
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "sync",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				return PostResult{Context: "sync-ctx"}, nil
			},
		}, Kind: KindPostTool},
		Entry{Hook: FuncHook{
			HookName: "audit",
			Post: func(_ context.Context, _ Event) (PostResult, error) {
				close(done)
				return PostResult{Context: "should-not-appear"}, nil
			},
		}, Kind: KindPostTool, Async: true},
	)

	out := m.PostTool(context.Background(), Event{Tool: "bash"})
	assert.Equal(t, "sync-ctx", out.Context)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async post hook did not run")
	}
}

func TestManagerSkipsWrongKind(t *testing.T) {
	var preCalled, postCalled atomic.Bool
	h := FuncHook{
		HookName: "pre-only-registered-as-post",
		Pre: func(_ context.Context, _ Event) (PreResult, error) {
			preCalled.Store(true)
			return PreResult{Action: ActionDeny}, nil
		},
		Post: func(_ context.Context, _ Event) (PostResult, error) {
			postCalled.Store(true)
			return PostResult{Context: "x"}, nil
		},
	}
	// Registered only as post — PreTool must not call Pre.
	m := NewManager(Entry{Hook: h, Kind: KindPostTool})
	pre := m.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.False(t, pre.Denied)
	assert.False(t, preCalled.Load())

	post := m.PostTool(context.Background(), Event{Tool: "bash"})
	assert.True(t, postCalled.Load())
	assert.Equal(t, "x", post.Context)
}

func TestManagerPostParallel(t *testing.T) {
	var gate sync.WaitGroup
	gate.Add(2)
	entered := make(chan struct{}, 2)

	m := NewManager(
		Entry{Hook: FuncHook{HookName: "p1", Post: func(_ context.Context, _ Event) (PostResult, error) {
			entered <- struct{}{}
			gate.Done()
			gate.Wait()
			return PostResult{Context: "a"}, nil
		}}, Kind: KindPostTool},
		Entry{Hook: FuncHook{HookName: "p2", Post: func(_ context.Context, _ Event) (PostResult, error) {
			entered <- struct{}{}
			gate.Done()
			gate.Wait()
			return PostResult{Context: "b"}, nil
		}}, Kind: KindPostTool},
	)

	done := make(chan PostOutcome, 1)
	go func() { done <- m.PostTool(context.Background(), Event{Tool: "bash"}) }()

	// Both must enter before either finishes — proves overlap.
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("hooks did not run in parallel")
		}
	}
	out := <-done
	assert.Contains(t, out.Context, "a")
	assert.Contains(t, out.Context, "b")
}

func TestManagerFailClosedOnlySkipsAuditHooks(t *testing.T) {
	var auditCalled, strictCalled atomic.Bool
	m := NewManager(
		Entry{Hook: FuncHook{
			HookName: "audit",
			Pre: func(_ context.Context, _ Event) (PreResult, error) {
				auditCalled.Store(true)
				return PreResult{Action: ActionAllow}, nil
			},
		}, Kind: KindPreTool},
		Entry{Hook: FuncHook{
			HookName: "strict",
			Pre: func(_ context.Context, _ Event) (PreResult, error) {
				strictCalled.Store(true)
				return PreResult{Action: ActionDeny, Reason: "nope"}, nil
			},
		}, Kind: KindPreTool, FailClosed: true},
	)

	full := m.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, full.Denied)
	assert.True(t, auditCalled.Load())
	assert.True(t, strictCalled.Load())

	auditCalled.Store(false)
	strictCalled.Store(false)
	out := m.FailClosedOnly().PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, out.Denied)
	assert.False(t, auditCalled.Load(), "non-fail_closed must be skipped in readonly view")
	assert.True(t, strictCalled.Load())
}

func TestNewManagerFiltersInvalid(t *testing.T) {
	m := NewManager(
		Entry{Hook: nil, Kind: KindPreTool},
		Entry{Hook: FuncHook{HookName: "ok"}, Kind: Kind("nope")},
		Entry{Hook: FuncHook{HookName: "pre", Pre: func(_ context.Context, _ Event) (PreResult, error) {
			return PreResult{Action: ActionDeny, Reason: "hit"}, nil
		}}, Kind: KindPreTool},
	)
	out := m.PreTool(context.Background(), Event{Tool: "bash", Input: json.RawMessage(`{}`)})
	assert.True(t, out.Denied)
	assert.Equal(t, "hit", out.Reason)
}

package hooks

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pulseaiclub/phi/internal/debuglog"
)

// MaxContextBytes caps aggregated hook Context injected back to the model.
const MaxContextBytes = 4 * 1024

// Kind selects which tool-loop phase an Entry participates in.
type Kind string

const (
	KindPreTool  Kind = "pre_tool"
	KindPostTool Kind = "post_tool"
)

// Entry wraps a Hook with per-registration metadata.
// FailClosed / Async stay off the Hook interface so in-process fakes stay small;
// directory discovery (S6) and CommandHook (S7) fill these fields.
type Entry struct {
	Hook       Hook
	Kind       Kind // KindPreTool or KindPostTool
	FailClosed bool
	Async      bool // Post only: fire-and-forget; result ignored
}

// Manager fans events out to registered entries.
//
// PreTool runs matching KindPreTool entries serially: first Deny wins;
// Modify chains onto Input. PostTool runs matching KindPostTool entries in
// parallel (except Async, which is detached). Call order across entries is
// not guaranteed — serialize logic inside one hook if order matters.
//
// Default failure mode is fail-open: hook errors / invalid Modify skip that
// entry. FailClosed turns those failures into Deny (Pre) or Stop (Post).
//
// A nil *Manager is safe and is a no-op.
//
// Readonly mode (permission.ModeReadonly) should call FailClosedOnly so
// exploratory tool loops are not stalled by slow audit hooks; security
// hooks keep FailClosed: true and still run.
type Manager struct {
	entries        []Entry
	failClosedOnly bool
}

// NewManager returns a manager over entries. Nil Hook entries are skipped.
func NewManager(entries ...Entry) *Manager {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Hook == nil {
			continue
		}
		if e.Kind != KindPreTool && e.Kind != KindPostTool {
			continue
		}
		out = append(out, e)
	}
	return &Manager{entries: out}
}

// FailClosedOnly returns a view that runs only FailClosed entries.
// Shares the underlying entry slice (read-only). Nil-safe.
func (m *Manager) FailClosedOnly() *Manager {
	if m == nil {
		return nil
	}
	return &Manager{entries: m.entries, failClosedOnly: true}
}

// PreOutcome is the aggregated PreTool decision for Executor.
type PreOutcome struct {
	Input   json.RawMessage
	Denied  bool
	Reason  string
	Context string
}

// PostOutcome is the aggregated PostTool decision for Executor.
type PostOutcome struct {
	Context string
	Stop    bool
	Reason  string
}

// PreTool runs pre_tool entries against ev. The returned Input should be used
// for Gate / Run (possibly modified). Denied means the tool must not run.
func (m *Manager) PreTool(ctx context.Context, ev Event) PreOutcome {
	if m == nil {
		return PreOutcome{Input: ev.Input}
	}
	out := PreOutcome{Input: ev.Input}
	var contexts []string

	for _, e := range m.entries {
		if e.Kind != KindPreTool {
			continue
		}
		if m.failClosedOnly && !e.FailClosed {
			continue
		}
		if !e.Hook.Match(ev.Tool) {
			continue
		}
		if ctx.Err() != nil {
			break
		}

		callEv := ev
		callEv.Input = out.Input
		res, err := e.Hook.PreTool(ctx, callEv)
		if err != nil {
			debuglog.Logf("hooks: %s PreTool: %v", e.Hook.Name(), err)
			if e.FailClosed {
				out.Denied = true
				out.Reason = failClosedReason(e.Hook.Name(), err)
				out.Context = joinContext(contexts)
				return out
			}
			continue
		}

		switch res.Action {
		case ActionDeny:
			out.Denied = true
			out.Reason = res.Reason
			if out.Reason == "" {
				out.Reason = "tool execution denied by hook " + e.Hook.Name()
			}
			if res.Context != "" {
				contexts = append(contexts, res.Context)
			}
			out.Context = joinContext(contexts)
			return out

		case ActionModify:
			if len(res.Input) == 0 {
				debuglog.Logf("hooks: %s PreTool modify with empty input", e.Hook.Name())
				if e.FailClosed {
					out.Denied = true
					out.Reason = "hook " + e.Hook.Name() + " modify returned empty input"
					out.Context = joinContext(contexts)
					return out
				}
				continue
			}
			out.Input = res.Input
			ev.Input = res.Input

		case ActionAllow:
			// ok
		default:
			debuglog.Logf("hooks: %s PreTool unknown action %v", e.Hook.Name(), res.Action)
			if e.FailClosed {
				out.Denied = true
				out.Reason = "hook " + e.Hook.Name() + " returned unknown action"
				out.Context = joinContext(contexts)
				return out
			}
		}

		if res.Context != "" {
			contexts = append(contexts, res.Context)
		}
	}

	out.Context = joinContext(contexts)
	return out
}

// PostTool runs post_tool entries against ev (after tool.Run).
func (m *Manager) PostTool(ctx context.Context, ev Event) PostOutcome {
	if m == nil {
		return PostOutcome{}
	}

	type result struct {
		res PostResult
		err error
		e   Entry
	}

	var syncEntries []Entry
	for _, e := range m.entries {
		if e.Kind != KindPostTool {
			continue
		}
		if m.failClosedOnly && !e.FailClosed {
			continue
		}
		if !e.Hook.Match(ev.Tool) {
			continue
		}
		if e.Async {
			go m.runPostAsync(e, ev)
			continue
		}
		syncEntries = append(syncEntries, e)
	}

	if len(syncEntries) == 0 {
		return PostOutcome{}
	}

	var wg sync.WaitGroup
	results := make([]result, len(syncEntries))
	for i, e := range syncEntries {
		wg.Add(1)
		go func(i int, e Entry) {
			defer wg.Done()
			res, err := e.Hook.PostTool(ctx, ev)
			results[i] = result{res: res, err: err, e: e}
		}(i, e)
	}
	wg.Wait()

	var (
		contexts []string
		reasons  []string
		stop     bool
	)
	for _, r := range results {
		if r.err != nil {
			debuglog.Logf("hooks: %s PostTool: %v", r.e.Hook.Name(), r.err)
			if r.e.FailClosed {
				stop = true
				reasons = append(reasons, failClosedReason(r.e.Hook.Name(), r.err))
			}
			continue
		}
		if r.res.Context != "" {
			contexts = append(contexts, r.res.Context)
		}
		if r.res.Stop {
			stop = true
			if r.res.Reason != "" {
				reasons = append(reasons, r.res.Reason)
			}
		}
	}

	return PostOutcome{
		Context: joinContext(contexts),
		Stop:    stop,
		Reason:  strings.Join(reasons, "; "),
	}
}

func (m *Manager) runPostAsync(e Entry, ev Event) {
	// Detach from the tool-call context so a finished turn does not abort audit hooks.
	if _, err := e.Hook.PostTool(context.Background(), ev); err != nil {
		debuglog.Logf("hooks: %s PostTool async: %v", e.Hook.Name(), err)
	}
}

func failClosedReason(name string, err error) string {
	return "hook " + name + " failed (fail_closed): " + err.Error()
}

func joinContext(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, "\n\n")
	if len(s) <= MaxContextBytes {
		return s
	}
	s = s[:MaxContextBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

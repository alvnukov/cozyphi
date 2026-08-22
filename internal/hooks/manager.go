package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pulseaiclub/phi/internal/debuglog"
)

// MaxContextBytes caps aggregated hook Context injected back to the model.
const MaxContextBytes = 4 * 1024

// Kind selects which hook event an Entry participates in.
type Kind string

// Kind values select which hook event an entry participates in.
const (
	KindPreTool             Kind = "pre_tool"
	KindPostTool            Kind = "post_tool"
	KindCommand             Kind = "command" // TUI slash command; not part of the tool loop
	KindSessionStart        Kind = "session_start"
	KindSessionShutdown     Kind = "session_shutdown"
	KindSessionBeforeSwitch Kind = "session_before_switch"
)

// Entry wraps a Hook with per-registration metadata.
// FailClosed / Async stay off the Hook interface so in-process fakes stay small;
// directory discovery and CommandHook fill these fields.
type Entry struct {
	Hook       Hook
	Kind       Kind
	FailClosed bool
	Async      bool // Post / session_start / session_shutdown: fire-and-forget
}

func validKind(k Kind) bool {
	switch k {
	case KindPreTool, KindPostTool, KindCommand,
		KindSessionStart, KindSessionShutdown, KindSessionBeforeSwitch:
		return true
	default:
		return false
	}
}

// NewManager returns a manager over entries. Nil Hook entries are skipped.
func NewManager(entries ...Entry) *Manager {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Hook == nil {
			continue
		}
		if !validKind(e.Kind) {
			continue
		}
		out = append(out, e)
	}
	return &Manager{entries: out}
}

// Manager fans events out to registered entries.
//
// PreTool runs matching KindPreTool entries serially: first Deny wins;
// Modify chains onto Input. PostTool runs matching KindPostTool entries in
// parallel (except Async, which is detached). Call order across entries is
// not guaranteed — serialize logic inside one hook if order matters.
//
// SessionBeforeSwitch runs serially; first Deny wins. SessionStart and
// SessionShutdown run in parallel (Async detached).
//
// Default failure mode is fail-open: hook errors / invalid Modify skip that
// entry. FailClosed turns those failures into Deny (Pre / before_switch) or
// Stop (Post).
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

// CommandEntries returns KindCommand entries. Nil-safe.
func (m *Manager) CommandEntries() []Entry {
	if m == nil {
		return nil
	}
	out := make([]Entry, 0, len(m.entries))
	for _, e := range m.entries {
		if e.Kind == KindCommand {
			out = append(out, e)
		}
	}
	return out
}

// RunCommand invokes the KindCommand hook named name (case-insensitive).
func (m *Manager) RunCommand(ctx context.Context, name string, ev CommandEvent) (CommandResult, error) {
	if m == nil {
		return CommandResult{}, errors.New("hooks: no manager")
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return CommandResult{}, errors.New("hooks: empty command name")
	}
	for _, e := range m.entries {
		if e.Kind != KindCommand {
			continue
		}
		if strings.ToLower(e.Hook.Name()) != key {
			continue
		}
		return e.Hook.Command(ctx, ev)
	}
	return CommandResult{}, fmt.Errorf("hooks: command %q is not registered", name)
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
	Output  string
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
			// Detach intentionally: a finished turn must not abort audit hooks.
			go m.runPostAsync(e, ev) //nolint:gosec // G118: async post hooks use Background on purpose
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
		output   string
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
		// Output rewrite is last-wins: the last matching hook in entry order
		// wins (execution is parallel, but the merge is sequential). Not run
		// through joinContext — that 4 KiB cap is for model-facing notes, not
		// tool result bodies.
		if r.res.Output != "" {
			output = r.res.Output
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
		Output:  output,
		Stop:    stop,
		Reason:  strings.Join(reasons, "; "),
	}
}

func (*Manager) runPostAsync(e Entry, ev Event) {
	// Detach from the tool-call context so a finished turn does not abort audit hooks.
	if _, err := e.Hook.PostTool(context.Background(), ev); err != nil {
		debuglog.Logf("hooks: %s PostTool async: %v", e.Hook.Name(), err)
	}
}

// SessionOutcome aggregates session lifecycle hook results for the Controller.
type SessionOutcome struct {
	Denied    bool
	Reason    string
	Toast     string
	Status    string
	StatusSet bool
}

// SessionBeforeSwitch runs session_before_switch entries serially. First Deny wins.
func (m *Manager) SessionBeforeSwitch(ctx context.Context, ev SessionEvent) SessionOutcome {
	ev.Kind = KindSessionBeforeSwitch
	return m.runSessionGate(ctx, KindSessionBeforeSwitch, ev)
}

// SessionStart runs session_start entries (parallel; Async detached).
func (m *Manager) SessionStart(ctx context.Context, ev SessionEvent) SessionOutcome {
	ev.Kind = KindSessionStart
	return m.runSessionNotify(ctx, KindSessionStart, ev)
}

// SessionShutdown runs session_shutdown entries (parallel; Async detached).
func (m *Manager) SessionShutdown(ctx context.Context, ev SessionEvent) SessionOutcome {
	ev.Kind = KindSessionShutdown
	return m.runSessionNotify(ctx, KindSessionShutdown, ev)
}

func (m *Manager) runSessionGate(ctx context.Context, kind Kind, ev SessionEvent) SessionOutcome {
	if m == nil {
		return SessionOutcome{}
	}
	var out SessionOutcome
	for _, e := range m.entries {
		if e.Kind != kind {
			continue
		}
		if m.failClosedOnly && !e.FailClosed {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		res, err := e.Hook.Session(ctx, ev)
		if err != nil {
			debuglog.Logf("hooks: %s Session: %v", e.Hook.Name(), err)
			if e.FailClosed {
				out.Denied = true
				out.Reason = failClosedReason(e.Hook.Name(), err)
				return out
			}
			continue
		}
		mergeSessionUI(&out, res)
		if res.Action == ActionDeny {
			out.Denied = true
			out.Reason = res.Reason
			if out.Reason == "" {
				out.Reason = "session switch denied by hook " + e.Hook.Name()
			}
			return out
		}
	}
	return out
}

func (m *Manager) runSessionNotify(ctx context.Context, kind Kind, ev SessionEvent) SessionOutcome {
	if m == nil {
		return SessionOutcome{}
	}

	type result struct {
		res SessionResult
		err error
		e   Entry
	}

	var syncEntries []Entry
	for _, e := range m.entries {
		if e.Kind != kind {
			continue
		}
		if m.failClosedOnly && !e.FailClosed {
			continue
		}
		if e.Async {
			go m.runSessionAsync(e, ev) //nolint:gosec // G118: async session hooks use Background on purpose
			continue
		}
		syncEntries = append(syncEntries, e)
	}
	if len(syncEntries) == 0 {
		return SessionOutcome{}
	}

	var wg sync.WaitGroup
	results := make([]result, len(syncEntries))
	for i, e := range syncEntries {
		wg.Add(1)
		go func(i int, e Entry) {
			defer wg.Done()
			res, err := e.Hook.Session(ctx, ev)
			results[i] = result{res: res, err: err, e: e}
		}(i, e)
	}
	wg.Wait()

	var out SessionOutcome
	for _, r := range results {
		if r.err != nil {
			debuglog.Logf("hooks: %s Session: %v", r.e.Hook.Name(), r.err)
			continue
		}
		mergeSessionUI(&out, r.res)
	}
	return out
}

func (*Manager) runSessionAsync(e Entry, ev SessionEvent) {
	if _, err := e.Hook.Session(context.Background(), ev); err != nil {
		debuglog.Logf("hooks: %s Session async: %v", e.Hook.Name(), err)
	}
}

func mergeSessionUI(out *SessionOutcome, res SessionResult) {
	if res.Toast != "" {
		out.Toast = res.Toast
	}
	if res.StatusSet {
		out.Status = res.Status
		out.StatusSet = true
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
	for s != "" && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

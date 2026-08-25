package controller

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
)

// RedrawRelay lets cmd construct a Bus before the Editor exists.
// Bind once after NewEditor; Fire is safe before Bind (no-op).
type RedrawRelay struct {
	fn atomic.Pointer[func()]
}

// NewRedrawRelay returns an empty relay.
func NewRedrawRelay() *RedrawRelay { return &RedrawRelay{} }

// Fire invokes the bound redraw callback, if any.
func (r *RedrawRelay) Fire() {
	if r == nil {
		return
	}
	if p := r.fn.Load(); p != nil && *p != nil {
		(*p)()
	}
}

// Bind sets the redraw callback (typically Editor.RequestRedraw).
func (r *RedrawRelay) Bind(fn func()) {
	if r == nil {
		return
	}
	if fn == nil {
		r.fn.Store(nil)
		return
	}
	r.fn.Store(new(fn))
}

// Bus is the single mailbox between components and the UI goroutine.
// Any goroutine may Publish; only the UI goroutine may Drain.
//
// Internally a buffered channel carries wake signals while a small queue
// holds messages so high-frequency stream events can coalesce.
// onWake (RequestRedraw) runs at most once per armed wake — coalesced
// publishes share a single redraw until Drain.
type Bus struct {
	mu      sync.Mutex
	pending []Msg
	wake    chan struct{}
	onWake  func()
}

// NewBus creates a mailbox. onWake is called when a wake is newly armed
// (e.g. RequestRedraw), not on every coalesced Publish.
func NewBus(onWake func()) *Bus {
	return &Bus{
		wake:   make(chan struct{}, 1),
		onWake: onWake,
	}
}

// Publish enqueues a message from any goroutine.
// AssistantMessageUpdate / same-tool ToolData / same child Progress coalesce
// even when not adjacent in the queue (latest wins).
func (b *Bus) Publish(m Msg) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if te, ok := m.(SessionEventMsg); ok {
		if i, ok := findCoalesceSession(b.pending, te); ok {
			b.pending[i] = te
			b.mu.Unlock()
			b.signal()
			return
		}
	}
	if jp, ok := m.(JobProgressMsg); ok {
		if i, ok := findCoalesceJobProgress(b.pending, jp); ok {
			b.pending[i] = jp
			b.mu.Unlock()
			b.signal()
			return
		}
	}
	b.pending = append(b.pending, m)
	b.mu.Unlock()
	b.signal()
}

func (b *Bus) signal() {
	select {
	case b.wake <- struct{}{}:
		if b.onWake != nil {
			b.onWake()
		}
	default:
		// Wake already armed; Drain will pick up all pending together.
	}
}

// Drain returns and clears the pending queue. UI goroutine only.
func (b *Bus) Drain() []Msg {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	batch := b.pending
	b.pending = nil
	b.mu.Unlock()
	// Clear wake signal so the next Publish can re-arm it.
	select {
	case <-b.wake:
	default:
	}
	return batch
}

// Chan exposes the wake signal for select-based loops (optional).
func (b *Bus) Chan() <-chan struct{} {
	if b == nil {
		return nil
	}
	return b.wake
}

func findCoalesceSession(pending []Msg, te SessionEventMsg) (int, bool) {
	if cur, ok := te.Event.(session.AssistantMessageUpdate); ok {
		for i := range slices.Backward(pending) {
			prev, ok := pending[i].(SessionEventMsg)
			if !ok {
				continue
			}
			prevUpd, ok := prev.Event.(session.AssistantMessageUpdate)
			if !ok {
				continue
			}
			// Only deltas of the SAME assistant row coalesce. A tool-round
			// boundary starts a new row id; matching on type alone let a new
			// round's first delta swallow the previous round's terminal event,
			// rewriting history above the user's queued message.
			if prevUpd.Message.ID == cur.Message.ID {
				return i, true
			}
		}
		return -1, false
	}
	td, ok := te.Event.(session.ToolData)
	if !ok {
		return -1, false
	}
	for i := range slices.Backward(pending) {
		prev, ok := pending[i].(SessionEventMsg)
		if !ok {
			continue
		}
		prevTD, ok := prev.Event.(session.ToolData)
		if ok && prevTD.Run.ToolUseID == td.Run.ToolUseID {
			return i, true
		}
	}
	return -1, false
}

func findCoalesceJobProgress(pending []Msg, jp JobProgressMsg) (int, bool) {
	for i := range slices.Backward(pending) {
		prev, ok := pending[i].(JobProgressMsg)
		if !ok {
			continue
		}
		if sameJobProgressSlot(prev.Progress, jp.Progress) {
			return i, true
		}
	}
	return -1, false
}

func sameJobProgressSlot(a, b job.Progress) bool {
	if a.JobID != b.JobID {
		return false
	}
	if a.ToolUseID != "" && b.ToolUseID != "" {
		return a.ToolUseID == b.ToolUseID
	}
	return a.Name == b.Name && a.Detail == b.Detail
}

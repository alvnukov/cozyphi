package tui

import (
	"sync"

	"github.com/pulseaiclub/phi/internal/session"
)

// Bus is the single mailbox between components and the UI goroutine.
// Any goroutine may Publish; only the UI goroutine may Drain.
//
// Internally a buffered channel carries wake signals while a small queue
// holds messages so high-frequency stream events can coalesce.
type Bus struct {
	mu      sync.Mutex
	pending []Msg
	wake    chan struct{}
	onWake  func()
}

// NewBus creates a mailbox. onWake is called after each Publish (e.g. RequestRedraw).
func NewBus(onWake func()) *Bus {
	return &Bus{
		wake:   make(chan struct{}, 1),
		onWake: onWake,
	}
}

// Publish enqueues a message from any goroutine.
// Consecutive AssistantMessageUpdate / same-tool ToolData / same child Progress coalesce.
func (b *Bus) Publish(m Msg) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if te, ok := m.(SessionEventMsg); ok {
		if coalesceSession(b.pending, te) {
			n := len(b.pending)
			b.pending[n-1] = te
			b.mu.Unlock()
			b.signal()
			return
		}
	}
	if jp, ok := m.(JobProgressMsg); ok {
		if coalesceJobProgress(b.pending, jp) {
			n := len(b.pending)
			b.pending[n-1] = jp
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
	default:
	}
	if b.onWake != nil {
		b.onWake()
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

func coalesceSession(pending []Msg, te SessionEventMsg) bool {
	n := len(pending)
	if n == 0 {
		return false
	}
	prev, ok := pending[n-1].(SessionEventMsg)
	if !ok {
		return false
	}
	if _, ok := te.Event.(session.AssistantMessageUpdate); ok {
		_, prevOK := prev.Event.(session.AssistantMessageUpdate)
		return prevOK
	}
	if td, ok := te.Event.(session.ToolData); ok {
		prevTD, prevOK := prev.Event.(session.ToolData)
		return prevOK && prevTD.Run.ToolUseID == td.Run.ToolUseID
	}
	return false
}

func coalesceJobProgress(pending []Msg, jp JobProgressMsg) bool {
	n := len(pending)
	if n == 0 {
		return false
	}
	prev, ok := pending[n-1].(JobProgressMsg)
	if !ok {
		return false
	}
	a, b := prev.Progress, jp.Progress
	if a.JobID != b.JobID {
		return false
	}
	if a.ToolUseID != "" && b.ToolUseID != "" {
		return a.ToolUseID == b.ToolUseID
	}
	return a.Name == b.Name && a.Detail == b.Detail
}

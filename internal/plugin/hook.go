package plugin

import (
	"context"
	"slices"
	"sync"

	"github.com/pulseaiclub/phi/internal/debuglog"
)

// Event is the payload handed to observational listeners.
type Event[T any] struct {
	Msg T
}

// Listener is invoked by [Hook.Emit] / [Hook.EmitAsync] / [Hook.EmitParallel].
type Listener[T any] func(ctx context.Context, event Event[T])

// Hook is a typed observational extension point (pub/sub, no result).
type Hook[T any] struct {
	name      string
	mu        sync.RWMutex
	listeners []keyedListener[T]
	nextID    uint64
	nextOrd   uint64
}

type keyedListener[T any] struct {
	id       uint64
	ord      uint64 // registration order for stable priority ties
	priority int
	once     bool
	fn       Listener[T]
}

// NewHook creates an observational hook point named name.
func NewHook[T any](name string) *Hook[T] {
	return &Hook[T]{name: name}
}

// Name returns the hook point name.
func (h *Hook[T]) Name() string { return h.name }

// Len returns the number of registered listeners.
func (h *Hook[T]) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.listeners)
}

// Clear removes all listeners.
func (h *Hook[T]) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners = nil
}

// On registers a listener. The returned function unregisters it.
// Passing a nil listener is a no-op and returns a no-op unsubscribe.
func (h *Hook[T]) On(listener Listener[T], opts ...SubOption) (unsubscribe func()) {
	if listener == nil {
		return func() {}
	}
	cfg := applySubOpts(opts)

	h.mu.Lock()
	h.nextID++
	h.nextOrd++
	id := h.nextID
	h.listeners = append(h.listeners, keyedListener[T]{
		id:       id,
		ord:      h.nextOrd,
		priority: cfg.priority,
		once:     cfg.once,
		fn:       listener,
	})
	h.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { h.remove(id) })
	}
}

func (h *Hook[T]) remove(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, l := range h.listeners {
		if l.id == id {
			h.listeners = append(h.listeners[:i], h.listeners[i+1:]...)
			return
		}
	}
}

// Emit invokes all listeners synchronously with ctx and message.
// Listeners are snapshotted under the lock so a listener may safely call On/Clear.
// A panic in one listener is recovered and does not stop the rest.
func (h *Hook[T]) Emit(ctx context.Context, message T) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, l := range h.snapshot() {
		if ctx.Err() != nil {
			return
		}
		if l.once {
			h.remove(l.id)
		}
		h.call(ctx, l.fn, message)
	}
}

// EmitAsync invokes each listener on its own goroutine.
// It does not wait for completion. Prefer Emit or EmitParallel when ordering
// or turn lifetime matters; use EmitAsync only for best-effort side effects.
func (h *Hook[T]) EmitAsync(ctx context.Context, message T) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Detach from caller cancel so a finished turn does not abort side effects
	// that already started; still refuse to schedule if ctx is already done.
	if ctx.Err() != nil {
		return
	}
	base := context.WithoutCancel(ctx)
	for _, l := range h.snapshot() {
		if l.once {
			h.remove(l.id)
		}
		fn := l.fn
		go h.call(base, fn, message)
	}
}

// EmitParallel invokes all listeners concurrently and waits for them to finish.
// Panics are recovered per listener and do not cancel the others.
func (h *Hook[T]) EmitParallel(ctx context.Context, message T) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	listeners := h.snapshot()
	if len(listeners) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, l := range listeners {
		if l.once {
			h.remove(l.id)
		}
		wg.Add(1)
		go func(fn Listener[T]) {
			defer wg.Done()
			h.call(ctx, fn, message)
		}(l.fn)
	}
	wg.Wait()
}

func (h *Hook[T]) snapshot() []keyedListener[T] {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.listeners) == 0 {
		return nil
	}
	out := make([]keyedListener[T], len(h.listeners))
	copy(out, h.listeners)
	slices.SortStableFunc(out, func(a, b keyedListener[T]) int {
		if a.priority != b.priority {
			return b.priority - a.priority // higher first
		}
		if a.ord < b.ord {
			return -1
		}
		if a.ord > b.ord {
			return 1
		}
		return 0
	})
	return out
}

func (h *Hook[T]) call(ctx context.Context, fn Listener[T], message T) {
	defer func() {
		if rec := recover(); rec != nil {
			debuglog.Logf("plugin: hook %q listener panic: %v", h.name, rec)
		}
	}()
	fn(ctx, Event[T]{Msg: message})
}

package plugin

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/pulseaiclub/phi/internal/debuglog"
)

// Handler produces a contribution to a [Chain] emit.
type Handler[T, R any] func(ctx context.Context, msg T) (R, error)

// Chain is a typed control-plane extension point: each handler returns R,
// results are folded with reduce in priority then registration order.
type Chain[T, R any] struct {
	name     string
	reduce   func(acc, next R) R
	stop     func(acc R) bool
	failOpen bool
	mu       sync.RWMutex
	handlers []keyedHandler[T, R]
	nextID   uint64
	nextOrd  uint64
}

type keyedHandler[T, R any] struct {
	id       uint64
	ord      uint64
	priority int
	once     bool
	fn       Handler[T, R]
}

// ChainOption configures [NewChain].
type ChainOption[R any] func(*chainConfig[R])

type chainConfig[R any] struct {
	stop     func(acc R) bool
	failOpen bool
}

// WithStop ends Emit early when stop(acc) is true after a reduce step.
func WithStop[R any](stop func(acc R) bool) ChainOption[R] {
	return func(c *chainConfig[R]) { c.stop = stop }
}

// WithFailOpen continues Emit when a handler returns an error (logs and skips
// that contribution). Default is fail-closed: the first error stops the chain.
func WithFailOpen[R any]() ChainOption[R] {
	return func(c *chainConfig[R]) { c.failOpen = true }
}

// NewChain creates a control-plane hook. reduce must be non-nil.
func NewChain[T, R any](name string, reduce func(acc, next R) R, opts ...ChainOption[R]) *Chain[T, R] {
	if reduce == nil {
		panic("plugin: NewChain requires non-nil reduce")
	}
	cfg := chainConfig[R]{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Chain[T, R]{
		name:     name,
		reduce:   reduce,
		stop:     cfg.stop,
		failOpen: cfg.failOpen,
	}
}

// Name returns the chain point name.
func (c *Chain[T, R]) Name() string { return c.name }

// Len returns the number of registered handlers.
func (c *Chain[T, R]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.handlers)
}

// Clear removes all handlers.
func (c *Chain[T, R]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = nil
}

// On registers a handler. The returned function unregisters it.
func (c *Chain[T, R]) On(handler Handler[T, R], opts ...SubOption) (unsubscribe func()) {
	if handler == nil {
		return func() {}
	}
	cfg := applySubOpts(opts)

	c.mu.Lock()
	c.nextID++
	c.nextOrd++
	id := c.nextID
	c.handlers = append(c.handlers, keyedHandler[T, R]{
		id:       id,
		ord:      c.nextOrd,
		priority: cfg.priority,
		once:     cfg.once,
		fn:       handler,
	})
	c.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() { c.remove(id) })
	}
}

func (c *Chain[T, R]) remove(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, h := range c.handlers {
		if h.id == id {
			c.handlers = append(c.handlers[:i], c.handlers[i+1:]...)
			return
		}
	}
}

// Emit runs handlers in priority then registration order, folding with reduce.
// initial is the seed accumulator.
//
// Default (fail-closed): the first handler error or panic stops the chain and
// is returned with the accumulator as of the last successful reduce.
// WithFailOpen: errors/panics are logged and skipped; Emit returns nil error.
func (c *Chain[T, R]) Emit(ctx context.Context, msg T, initial R) (R, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	acc := initial
	for _, h := range c.snapshot() {
		if err := ctx.Err(); err != nil {
			return acc, err
		}
		if h.once {
			c.remove(h.id)
		}
		next, err := c.call(ctx, h.fn, msg)
		if err != nil {
			if c.failOpen {
				debuglog.Logf("plugin: chain %q handler: %v", c.name, err)
				continue
			}
			return acc, err
		}
		acc = c.reduce(acc, next)
		if c.stop != nil && c.stop(acc) {
			return acc, nil
		}
	}
	return acc, nil
}

func (c *Chain[T, R]) snapshot() []keyedHandler[T, R] {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.handlers) == 0 {
		return nil
	}
	out := make([]keyedHandler[T, R], len(c.handlers))
	copy(out, c.handlers)
	slices.SortStableFunc(out, func(a, b keyedHandler[T, R]) int {
		if a.priority != b.priority {
			return b.priority - a.priority
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

func (c *Chain[T, R]) call(ctx context.Context, fn Handler[T, R], msg T) (next R, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("plugin: chain %q handler panic: %v", c.name, rec)
		}
	}()
	return fn(ctx, msg)
}

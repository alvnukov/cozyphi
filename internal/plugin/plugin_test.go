package plugin

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHookEmitSnapshotsListeners(t *testing.T) {
	h := NewHook[string]("t")
	var saw []string
	h.On(func(_ context.Context, e Event[string]) {
		saw = append(saw, e.Msg)
		// Registering during Emit must not deadlock or re-enter this emit.
		h.On(func(context.Context, Event[string]) {})
	})
	h.Emit(t.Context(), "a")
	assert.Equal(t, []string{"a"}, saw)
	assert.Equal(t, 2, h.Len())
}

func TestHookUnsubscribe(t *testing.T) {
	h := NewHook[int]("t")
	var n atomic.Int32
	off := h.On(func(context.Context, Event[int]) { n.Add(1) })
	h.Emit(t.Context(), 1)
	off()
	off() // idempotent
	h.Emit(t.Context(), 2)
	assert.Equal(t, int32(1), n.Load())
	assert.Equal(t, 0, h.Len())
}

func TestHookEmitStopsOnCancel(t *testing.T) {
	h := NewHook[int]("t")
	var n atomic.Int32
	h.On(func(context.Context, Event[int]) { n.Add(1) })
	h.On(func(context.Context, Event[int]) { n.Add(1) })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h.Emit(ctx, 1)
	assert.Equal(t, int32(0), n.Load())
}

func TestHookRecoverPanic(t *testing.T) {
	h := NewHook[string]("t")
	var ok atomic.Bool
	h.On(func(context.Context, Event[string]) { panic("boom") })
	h.On(func(context.Context, Event[string]) { ok.Store(true) })
	h.Emit(t.Context(), "x")
	assert.True(t, ok.Load())
}

func TestHookEmitAsync(t *testing.T) {
	h := NewHook[string]("t")
	var wg sync.WaitGroup
	wg.Add(1)
	h.On(func(context.Context, Event[string]) { defer wg.Done() })
	h.EmitAsync(t.Context(), "x")
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async listener did not run")
	}
}

func TestHookPriorityAndFIFO(t *testing.T) {
	h := NewHook[string]("t")
	var order []string
	h.On(func(context.Context, Event[string]) { order = append(order, "low") }, WithPriority(1))
	h.On(func(context.Context, Event[string]) { order = append(order, "a") }, WithPriority(10))
	h.On(func(context.Context, Event[string]) { order = append(order, "b") }, WithPriority(10))
	h.On(func(context.Context, Event[string]) { order = append(order, "mid") }, WithPriority(5))
	h.Emit(t.Context(), "x")
	assert.Equal(t, []string{"a", "b", "mid", "low"}, order)
}

func TestHookOnce(t *testing.T) {
	h := NewHook[int]("t")
	var n atomic.Int32
	h.On(func(context.Context, Event[int]) { n.Add(1) }, WithOnce())
	h.Emit(t.Context(), 1)
	h.Emit(t.Context(), 2)
	assert.Equal(t, int32(1), n.Load())
	assert.Equal(t, 0, h.Len())
}

func TestHookClear(t *testing.T) {
	h := NewHook[int]("t")
	h.On(func(context.Context, Event[int]) {})
	h.On(func(context.Context, Event[int]) {})
	h.Clear()
	assert.Equal(t, 0, h.Len())
	var n atomic.Int32
	h.On(func(context.Context, Event[int]) { n.Add(1) })
	h.Clear()
	h.Emit(t.Context(), 1)
	assert.Equal(t, int32(0), n.Load())
}

func TestHookEmitParallel(t *testing.T) {
	h := NewHook[string]("t")
	var n atomic.Int32
	h.On(func(context.Context, Event[string]) {
		time.Sleep(20 * time.Millisecond)
		n.Add(1)
	})
	h.On(func(context.Context, Event[string]) {
		time.Sleep(20 * time.Millisecond)
		n.Add(1)
	})
	h.On(func(context.Context, Event[string]) { panic("boom") })

	start := time.Now()
	h.EmitParallel(t.Context(), "x")
	elapsed := time.Since(start)

	assert.Equal(t, int32(2), n.Load())
	assert.Less(t, elapsed, 100*time.Millisecond, "listeners should run concurrently")
}

func TestChainReduceAndStop(t *testing.T) {
	type out struct {
		sum  int
		stop bool
	}
	c := NewChain[int, out]("sum",
		func(acc, next out) out {
			return out{sum: acc.sum + next.sum, stop: acc.stop || next.stop}
		},
		WithStop(func(acc out) bool { return acc.stop }),
	)

	c.On(func(context.Context, int) (out, error) { return out{sum: 1}, nil })
	c.On(func(context.Context, int) (out, error) { return out{sum: 2, stop: true}, nil })
	var third atomic.Bool
	c.On(func(context.Context, int) (out, error) {
		third.Store(true)
		return out{sum: 99}, nil
	})

	got, err := c.Emit(t.Context(), 0, out{})
	require.NoError(t, err)
	assert.Equal(t, 3, got.sum)
	assert.True(t, got.stop)
	assert.False(t, third.Load())
}

func TestChainHandlerError(t *testing.T) {
	c := NewChain[string, int]("n", func(acc, next int) int { return acc + next })
	c.On(func(context.Context, string) (int, error) { return 1, nil })
	c.On(func(context.Context, string) (int, error) { return 0, errors.New("nope") })
	c.On(func(context.Context, string) (int, error) { return 5, nil })

	got, err := c.Emit(t.Context(), "x", 0)
	assert.Equal(t, 1, got)
	assert.EqualError(t, err, "nope")
}

func TestChainFailOpen(t *testing.T) {
	c := NewChain[string, int]("n",
		func(acc, next int) int { return acc + next },
		WithFailOpen[int](),
	)
	c.On(func(context.Context, string) (int, error) { return 1, nil })
	c.On(func(context.Context, string) (int, error) { return 0, errors.New("nope") })
	c.On(func(context.Context, string) (int, error) { return 5, nil })

	got, err := c.Emit(t.Context(), "x", 0)
	require.NoError(t, err)
	assert.Equal(t, 6, got)
}

func TestChainPriority(t *testing.T) {
	c := NewChain[string, []string]("n",
		func(acc, next []string) []string { return append(acc, next...) },
	)
	c.On(func(context.Context, string) ([]string, error) {
		return []string{"low"}, nil
	}, WithPriority(1))
	c.On(func(context.Context, string) ([]string, error) {
		return []string{"high"}, nil
	}, WithPriority(10))

	got, err := c.Emit(t.Context(), "x", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"high", "low"}, got)
}

func TestChainOnce(t *testing.T) {
	c := NewChain[string, int]("n", func(acc, next int) int { return acc + next })
	c.On(func(context.Context, string) (int, error) { return 1, nil }, WithOnce())
	got, err := c.Emit(t.Context(), "x", 0)
	require.NoError(t, err)
	assert.Equal(t, 1, got)
	got, err = c.Emit(t.Context(), "x", 0)
	require.NoError(t, err)
	assert.Equal(t, 0, got)
	assert.Equal(t, 0, c.Len())
}

func TestChainPanicBecomesError(t *testing.T) {
	c := NewChain[string, int]("n", func(acc, next int) int { return acc + next })
	c.On(func(context.Context, string) (int, error) { panic("x") })
	_, err := c.Emit(t.Context(), "x", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
}

func TestNewChainNilReducePanics(t *testing.T) {
	assert.Panics(t, func() {
		NewChain[int, int]("x", nil)
	})
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	h := NewHook[int]("tool_done")
	c := NewChain[string, int]("pre_tool", func(acc, next int) int { return acc + next })

	require.NoError(t, r.Add(h))
	require.NoError(t, r.Add(c))
	assert.EqualError(t, r.Add(NewHook[int]("tool_done")), `plugin: duplicate point "tool_done"`)
	assert.Panics(t, func() { r.MustAdd(NewHook[int]("tool_done")) })

	p, ok := r.Get("pre_tool")
	require.True(t, ok)
	assert.Equal(t, "pre_tool", p.Name())

	assert.Equal(t, []string{"pre_tool", "tool_done"}, r.Names())
	assert.Equal(t, 2, r.Len())

	h.On(func(context.Context, Event[int]) {})
	c.On(func(context.Context, string) (int, error) { return 1, nil })
	assert.Equal(t, 1, h.Len())
	assert.Equal(t, 1, c.Len())

	r.ClearAll()
	assert.Equal(t, 0, r.Len())
	assert.Equal(t, 0, h.Len())
	assert.Equal(t, 0, c.Len())
	_, ok = r.Get("tool_done")
	assert.False(t, ok)
}

func TestRegistryNilSafe(t *testing.T) {
	var r *Registry
	assert.Equal(t, 0, r.Len())
	assert.Nil(t, r.Names())
	_, ok := r.Get("x")
	assert.False(t, ok)
	r.ClearAll()
	assert.Error(t, r.Add(NewHook[int]("x")))
}

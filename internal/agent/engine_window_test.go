package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
)

func newWindowEngine(t *testing.T, window int) *Engine {
	t.Helper()
	dir := t.TempDir()
	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "m", APIKey: "k", BaseURL: "http://example", ContextWindow: window},
		SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: false},
	})
	require.NoError(t, err)
	return engine
}

func TestSetContextWindowOverride(t *testing.T) {
	t.Run("narrows below the model window", func(t *testing.T) {
		engine := newWindowEngine(t, 200000)
		engine.SetContextWindowOverride(32000)
		assert.Equal(t, 32000, engine.ContextWindow())
	})

	t.Run("clamps above the model window", func(t *testing.T) {
		engine := newWindowEngine(t, 100000)
		engine.SetContextWindowOverride(500000)
		assert.Equal(t, 100000, engine.ContextWindow())
	})

	t.Run("zero restores the model window", func(t *testing.T) {
		engine := newWindowEngine(t, 200000)
		engine.SetContextWindowOverride(32000)
		engine.SetContextWindowOverride(0)
		assert.Equal(t, 200000, engine.ContextWindow())
	})

	t.Run("negative is treated as no override", func(t *testing.T) {
		engine := newWindowEngine(t, 200000)
		engine.SetContextWindowOverride(-5)
		assert.Equal(t, 200000, engine.ContextWindow())
	})

	t.Run("survives a model switch", func(t *testing.T) {
		engine := newWindowEngine(t, 200000)
		engine.SetContextWindowOverride(32000)
		require.NoError(t, engine.SetModel(llm.ModelConfig{
			Name: "m2", APIKey: "k", BaseURL: "http://example", ContextWindow: 1000000,
		}))
		assert.Equal(t, 32000, engine.ContextWindow())
		// A smaller model window wins over a larger override: the override
		// narrows, it never promises headroom the provider refuses.
		require.NoError(t, engine.SetModel(llm.ModelConfig{
			Name: "m3", APIKey: "k", BaseURL: "http://example", ContextWindow: 16000,
		}))
		assert.Equal(t, 16000, engine.ContextWindow())
	})

	t.Run("model without a window trusts the override", func(t *testing.T) {
		engine := newWindowEngine(t, 0)
		engine.SetContextWindowOverride(64000)
		assert.Equal(t, 64000, engine.ContextWindow())
	})
}

func TestEngineRunnerContextLimit(t *testing.T) {
	dir := t.TempDir()
	base := llm.ModelConfig{Name: "m", APIKey: "k", BaseURL: "http://example", ContextWindow: 200000}

	build := func(limit func() int) (*Engine, error) {
		runner := EngineRunner{Model: base, ContextLimit: limit}
		engine, _, err := runner.buildChild(job.Meta{Role: "explore", Dir: dir, WorkDir: dir})
		return engine, err
	}

	t.Run("narrows the child window", func(t *testing.T) {
		engine, err := build(func() int { return 32000 })
		require.NoError(t, err)
		assert.Equal(t, 32000, engine.ContextWindow())
	})

	t.Run("never widens a smaller window", func(t *testing.T) {
		engine, err := build(func() int { return 500000 })
		require.NoError(t, err)
		assert.Equal(t, 200000, engine.ContextWindow())
	})

	t.Run("zero or nil leaves the window alone", func(t *testing.T) {
		engine, err := build(func() int { return 0 })
		require.NoError(t, err)
		assert.Equal(t, 200000, engine.ContextWindow())

		engine, err = build(nil)
		require.NoError(t, err)
		assert.Equal(t, 200000, engine.ContextWindow())
	})
}

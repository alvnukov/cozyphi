package agent

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngineResumeResolvesSessionModel(t *testing.T) {
	dir := t.TempDir()
	first, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
	})
	require.NoError(t, err)

	require.NoError(t, first.session.Append(llm.Message{Role: llm.RoleUser, Content: "hi"}))
	require.NoError(
		t,
		first.session.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "yo"}, "model-last", ""),
	)

	resumeCfg := llm.ModelConfig{Name: "model-last", APIKey: "k2", BaseURL: "http://example2", ContextWindow: 4096}
	require.NoError(t, first.Session().Close())
	second, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
		SessionOpts: SessionOpts{
			ResumePath: first.SessionFile(),
		},
		ResolveModel: func(name string) (llm.ModelConfig, bool) {
			if name == "model-last" {
				return resumeCfg, true
			}
			return llm.ModelConfig{}, false
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Session().Close()) })

	assert.Equal(t, "model-last", second.ModelConfig().Name)
	assert.Equal(t, 4096, second.contextWindow)
}

func TestNewEngineResumeKeepsDefaultWhenNoSessionModel(t *testing.T) {
	dir := t.TempDir()
	first, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
	})
	require.NoError(t, err)
	require.NoError(t, first.session.Append(llm.Message{Role: llm.RoleUser, Content: "hi"}))
	require.NoError(t, first.session.Append(llm.Message{Role: llm.RoleAssistant, Content: "yo"}))

	require.NoError(t, first.Session().Close())
	second, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
		SessionOpts: SessionOpts{
			ResumePath: first.SessionFile(),
		},
		ResolveModel: func(name string) (llm.ModelConfig, bool) {
			t.Fatalf("unexpected resolve call for %q", name)
			return llm.ModelConfig{}, false
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Session().Close()) })

	assert.Equal(t, "model-default", second.ModelConfig().Name)
}

// TestNewEngineResumeRestoresSessionEffort: a session records the reasoning
// effort it last ran with (AppendAssistant), so a resume must pick it up
// again — not silently fall back to the resolved model's base default.
// The recorded level rides the resolved config the same way a plan-step
// override does: unsupported or empty keeps the base default.
func TestNewEngineResumeRestoresSessionEffort(t *testing.T) {
	newSessionRecordingEffort := func(t *testing.T, model, effort string) string {
		t.Helper()
		dir := t.TempDir()
		first, err := NewEngine(EngineOpts{
			Model:       llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
			SessionOpts: SessionOpts{Cwd: dir, SessionDir: dir, Persist: true},
		})
		require.NoError(t, err)
		require.NoError(t, first.session.Append(llm.Message{Role: llm.RoleUser, Content: "hi"}))
		require.NoError(
			t,
			first.session.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "yo"}, model, effort),
		)
		file := first.SessionFile()
		require.NoError(t, first.Session().Close())
		return file
	}
	resolveCfg := llm.ModelConfig{
		Name: "model-last", APIKey: "k2", BaseURL: "http://example2",
		ReasoningEffort:  llm.ReasoningEffortMedium,
		ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
	}

	t.Run("recorded effort restores", func(t *testing.T) {
		second, err := NewEngine(EngineOpts{
			Model:        llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
			SessionOpts:  SessionOpts{ResumePath: newSessionRecordingEffort(t, "model-last", "high")},
			ResolveModel: func(name string) (llm.ModelConfig, bool) { return resolveCfg, name == "model-last" },
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, second.Session().Close()) })
		assert.Equal(t, llm.ReasoningEffortHigh, second.ModelConfig().ReasoningEffort,
			"the session's recorded effort must not fall back to the base default")
	})

	t.Run("unsupported recorded effort keeps the base default", func(t *testing.T) {
		second, err := NewEngine(EngineOpts{
			Model:        llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
			SessionOpts:  SessionOpts{ResumePath: newSessionRecordingEffort(t, "model-last", "xhigh")},
			ResolveModel: func(name string) (llm.ModelConfig, bool) { return resolveCfg, name == "model-last" },
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, second.Session().Close()) })
		assert.Equal(t, llm.ReasoningEffortMedium, second.ModelConfig().ReasoningEffort)
	})

	t.Run("default-effort session keeps the base default", func(t *testing.T) {
		second, err := NewEngine(EngineOpts{
			Model:        llm.ModelConfig{Name: "model-default", APIKey: "k", BaseURL: "http://example"},
			SessionOpts:  SessionOpts{ResumePath: newSessionRecordingEffort(t, "model-last", "")},
			ResolveModel: func(name string) (llm.ModelConfig, bool) { return resolveCfg, name == "model-last" },
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, second.Session().Close()) })
		assert.Equal(t, llm.ReasoningEffortMedium, second.ModelConfig().ReasoningEffort)
	})
}

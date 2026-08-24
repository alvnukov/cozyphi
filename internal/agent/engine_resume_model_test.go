package agent

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"

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
	require.NoError(t, first.session.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "yo"}, "model-last"))

	resumeCfg := llm.ModelConfig{Name: "model-last", APIKey: "k2", BaseURL: "http://example2", ContextWindow: 4096}
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

	assert.Equal(t, "model-default", second.ModelConfig().Name)
}

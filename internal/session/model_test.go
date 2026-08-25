package session

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerModel(t *testing.T) {
	t.Run("header model without assistant entries", func(t *testing.T) {
		manager, err := NewSessionManager(t.TempDir(),
			WithSessionDir(t.TempDir()),
			WithShouldFlush(false),
			WithModel("gpt-4o"),
		)
		require.NoError(t, err)
		assert.Equal(t, "gpt-4o", manager.Model())
	})

	t.Run("last assistant model wins", func(t *testing.T) {
		manager, err := NewSessionManager(t.TempDir(),
			WithSessionDir(t.TempDir()),
			WithShouldFlush(false),
			WithModel("gpt-4o"),
		)
		require.NoError(t, err)

		_, err = manager.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "one"}, "claude-3-5-sonnet")
		require.NoError(t, err)
		_, err = manager.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "two"}, "codex/gpt-5.2:high")
		require.NoError(t, err)

		assert.Equal(t, "codex/gpt-5.2:high", manager.Model())
	})

	t.Run("survives persist and reload", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := NewSessionManager(dir,
			WithSessionDir(dir),
			WithShouldFlush(true),
			WithModel("gpt-4o"),
		)
		require.NoError(t, err)

		_, err = manager.Append(llm.Message{Role: llm.RoleUser, Content: "hi"})
		require.NoError(t, err)
		_, err = manager.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "hello"}, "claude-3-5-sonnet")
		require.NoError(t, err)

		reloaded, err := OpenSession(manager.File())
		require.NoError(t, err)
		assert.Equal(t, "claude-3-5-sonnet", reloaded.Model())
	})
}

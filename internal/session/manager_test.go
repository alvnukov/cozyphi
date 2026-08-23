package session

import (
	"os"
	"testing"

	"github.com/pulseaiclub/phi/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSessionManager(t *testing.T) {
	t.Run("no flush", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := NewSessionManager(
			dir,
			WithSessionDir(dir),
			WithShouldFlush(true),
		)
		require.NoError(t, err)
		assert.Equal(t, dir, manager.cwd)
		assert.Equal(t, dir, manager.config.sessionDir)
		assert.True(t, manager.config.shouldFlush)
		assert.False(t, manager.flushed)
		assert.Nil(t, manager.leafID)
		assert.NotEmpty(t, manager.sessionID)
		assert.NotEmpty(t, manager.sessionFile)
	})
}

func TestGetBranch(t *testing.T) {
	dir := t.TempDir()
	manager, err := NewSessionManager(
		dir,
		WithSessionDir(dir),
		WithShouldFlush(false),
	)
	require.NoError(t, err)

	id1, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "Hello",
	})
	require.NoError(t, err)

	id2, err := manager.Append(llm.Message{
		Role:    llm.RoleAssistant,
		Content: "Hi there!",
	})
	require.NoError(t, err)

	id3, err := manager.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: "How are you?",
	})
	require.NoError(t, err)

	branch := manager.GetBranch(id3)
	require.Len(t, branch, 3)

	assert.Equal(t, id1, branch[2].GetID())
	assert.Equal(t, id2, branch[1].GetID())
	assert.Equal(t, id3, branch[0].GetID())
}

func TestAppendEntry(t *testing.T) {
	t.Run("flush to disk", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := NewSessionManager(
			dir,
			WithSessionDir(dir),
			WithShouldFlush(true),
		)
		require.NoError(t, err)

		_, err = manager.Append(llm.Message{
			Role:    llm.RoleUser,
			Content: "Hello",
		})
		require.NoError(t, err)

		_, err = manager.Append(llm.Message{
			Role:    llm.RoleAssistant,
			Content: "Hi there!",
		})
		require.NoError(t, err)

		assert.True(t, manager.flushed)

		info, err := os.Stat(manager.sessionFile)
		require.NoError(t, err, "session file should be created")
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "session file should be owner-only")
	})

	t.Run("no flush to disk", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := NewSessionManager(
			dir,
			WithSessionDir(dir),
			WithShouldFlush(false),
		)
		require.NoError(t, err)

		id, err := manager.Append(llm.Message{
			Role:    llm.RoleUser,
			Content: "test",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.False(t, manager.flushed)
	})
}

func TestBuildSessionContext(t *testing.T) {
	t.Run("no compaction, returns all messages in order", func(t *testing.T) {
		entry1 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type: EntryMessage,
				ID:   "msg1",
			},
			Message: llm.Message{
				Role:    llm.RoleUser,
				Content: "hello",
			},
		}
		entry2 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type:     EntryMessage,
				ID:       "msg2",
				ParentID: &entry1.ID,
			},
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "hi",
			},
		}
		entry3 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type:     EntryMessage,
				ID:       "msg3",
				ParentID: &entry2.ID,
			},
			Message: llm.Message{
				Role:    llm.RoleUser,
				Content: "how are you?",
			},
		}

		entries := []MessageEntry{entry1, entry2, entry3}
		byID := map[string]MessageEntry{
			entry1.ID: entry1,
			entry2.ID: entry2,
			entry3.ID: entry3,
		}

		ctx := buildSessionContext(entries, entry3.ID, byID)
		require.Len(t, ctx, 3)

		assert.Equal(t, "msg1", ctx[0].GetID())
		assert.Equal(t, "msg2", ctx[1].GetID())
		assert.Equal(t, "msg3", ctx[2].GetID())
	})

	t.Run("with compaction, includes compaction and subsequent messages", func(t *testing.T) {
		entry1 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type: EntryMessage,
				ID:   "msg1",
			},
			Message: llm.Message{
				Role:    llm.RoleUser,
				Content: "hello",
			},
		}
		entry2 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type:     EntryMessage,
				ID:       "msg2",
				ParentID: &entry1.ID,
			},
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "hi",
			},
		}
		compaction := CompactionEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type:     EntryCompaction,
				ID:       "cmp1",
				ParentID: &entry2.ID,
			},
			Compaction: Compaction{
				Summary:          "summary",
				FirstKeptEntryID: "msg2",
			},
		}
		entry3 := SessionMessageEntry{
			SessionBaseEntry: SessionBaseEntry{
				Type:     EntryMessage,
				ID:       "msg3",
				ParentID: &compaction.ID,
			},
			Message: llm.Message{
				Role:    llm.RoleUser,
				Content: "after compaction",
			},
		}

		entries := []MessageEntry{entry1, entry2, compaction, entry3}
		byID := map[string]MessageEntry{
			entry1.ID:     entry1,
			entry2.ID:     entry2,
			compaction.ID: compaction,
			entry3.ID:     entry3,
		}

		ctx := buildSessionContext(entries, entry3.ID, byID)

		require.Len(t, ctx, 3)
		assert.Equal(t, EntryCompaction, ctx[0].GetType())
		assert.Equal(t, "cmp1", ctx[0].GetID())
		assert.Equal(t, "msg2", ctx[1].GetID())
		assert.Equal(t, "msg3", ctx[2].GetID())
	})
}

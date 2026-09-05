package session_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestDeliveryReceiptAndContextPersistTogether(t *testing.T) {
	manager, err := session.NewSessionManager(
		t.TempDir(),
		session.WithSessionDir(t.TempDir()),
		session.WithShouldFlush(true),
	)
	require.NoError(t, err)
	message := llm.Message{
		Role:    llm.RoleUser,
		Content: "<system-reminder>Child output, not user input: done</system-reminder>",
	}
	added, err := manager.AppendDelivery("job-one:terminal", message)
	require.NoError(t, err)
	require.True(t, added)
	reopened, err := session.OpenSession(manager.File())
	require.NoError(t, err, "a delivery persists even before any assistant answer")
	added, err = reopened.AppendDelivery("job-one:terminal", message)
	require.NoError(t, err)
	require.False(t, added, "retry does not append another model-context message")
	require.Len(t, reopened.BuildContext(), 1)
	_, err = reopened.Append(llm.Message{Role: llm.RoleUser, Content: "next question"})
	require.NoError(t, err)
	again, err := session.OpenSession(manager.File())
	require.NoError(t, err)
	require.Len(t, again.BuildContext(), 2, "messages following the first durable delivery also persist")
}

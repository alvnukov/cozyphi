package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSessionListLineMarksActive(t *testing.T) {
	dir := t.TempDir()
	path := writeTUISession(t, dir, "active123", time.Now())
	m, err := session.OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	list, err := session.ListSessions(dir)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Contains(t, sessionListLine(list[0]), "active123 [active]")
	require.NoError(t, m.Close())
	list, err = session.ListSessions(dir)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotContains(t, sessionListLine(list[0]), "[active]")
}

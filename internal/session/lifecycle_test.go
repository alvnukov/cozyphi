package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Persistence tests transfer ownership, rather than opening a second writer to
// inspect disk. The old manager stays readable but must not be mutated again.
func reopenSession(t *testing.T, m *Manager) (*Manager, error) {
	t.Helper()
	require.NoError(t, m.Close())
	loaded, err := OpenSession(m.File())
	if err == nil {
		t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	}
	return loaded, err
}

func newTestSessionManager(t *testing.T, path string, opts ...ManagerOption) (*Manager, error) {
	t.Helper()
	m, err := NewSessionManager(path, opts...)
	if err == nil {
		t.Cleanup(func() { require.NoError(t, m.Close()) })
	}
	return m, err
}

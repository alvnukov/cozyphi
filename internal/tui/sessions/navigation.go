package sessions

import (
	"fmt"
	"path/filepath"

	"github.com/alvnukov/cozyphi/internal/session"
)

type sessionNavigation struct {
	registry *Registry
	activate func(string) error
}

// ConfigureSessionNavigation routes /resume of retained history through shell
// selection. Other histories still use the controller's transactional resume.
func (e *View) ConfigureSessionNavigation(registry *Registry, activate func(string) error) {
	e.navigation = &sessionNavigation{registry: registry, activate: activate}
}

func (e *View) selectRetainedSession(id string) (bool, error) {
	n := e.navigation
	if n == nil || n.registry == nil || n.activate == nil || e.ctrl == nil {
		return false, nil
	}
	// Resolve against the complete history, not only retained entries: a prefix
	// ambiguous on disk must not silently select one of the live owners.
	path, err := session.FindSessionFile(e.ctrl.SessionDir(), id)
	if err != nil {
		return false, err
	}
	path, err = filepath.Abs(path)
	if err == nil {
		path, err = filepath.EvalSymlinks(path)
	}
	if err != nil {
		return false, fmt.Errorf("resolve retained session: %w", err)
	}
	for _, entry := range n.registry.Entries() {
		view := entry.View
		if view.ctrl != nil && !view.lifetime.closed && view.ctrl.SessionFile() == path {
			return true, n.activate(entry.ID)
		}
	}
	return false, nil
}

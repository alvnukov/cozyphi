package shelltask

import (
	"errors"
	"fmt"
	"os"

	"github.com/alvnukov/cozyphi/internal/proc"
)

func createOutput(dir, path string) (*os.File, error) {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create shell task: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create shell output: %w", err)
	}
	return file, nil
}

// takeResult transfers the only full output copy to the foreground caller.
// Background history retains only metadata and the bounded live tail.
func (m *Manager) takeResult(e *entry) (Result, error) {
	m.mu.Lock()
	result, err := Result{Snapshot: e.snapshot, Process: e.result}, e.runErr
	e.result = proc.Result{}
	if e.snapshot.Background {
		m.mu.Unlock()
		return Result{Snapshot: result.Snapshot}, nil
	}
	m.removeLocked(e)
	m.signalLocked()
	m.mu.Unlock()
	if removeErr := os.RemoveAll(e.dir); removeErr != nil {
		err = errors.Join(err, fmt.Errorf("remove foreground shell artifacts: %w", removeErr))
	}
	return result, err
}

// reserveBackgroundLocked evicts only acknowledged terminal history. Pending
// outcomes cannot be discarded to make room for new work. Foreground work does
// not consume history capacity.
func (m *Manager) reserveBackgroundLocked() error {
	if m.cleanupErr != nil {
		return fmt.Errorf(
			"background artifact cleanup failed; repair the task store before starting more background work: %w",
			m.cleanupErr,
		)
	}
	count := 0
	var oldest *entry
	for _, id := range m.order {
		e := m.entries[id]
		if !e.snapshot.Background {
			continue
		}
		count++
		if oldest == nil && e.snapshot.State.Terminal() && e.acknowledged {
			oldest = e
		}
	}
	if count < MaxTasks {
		return nil
	}
	if oldest == nil {
		return fmt.Errorf(
			"background history is full: %d undelivered or running tasks; allow delivery before starting more background work",
			MaxTasks,
		)
	}
	select {
	case m.cleanup <- oldest.dir:
		m.removeLocked(oldest)
		return nil
	default:
		return errors.New("shell artifact cleanup is busy; wait before starting more background work")
	}
}

func (m *Manager) cleanArtifacts() {
	defer m.cleanupWG.Done()
	for dir := range m.cleanup {
		if err := os.RemoveAll(dir); err != nil {
			m.mu.Lock()
			if m.cleanupErr == nil {
				m.cleanupErr = err
			}
			m.cleanupFailures++
			m.mu.Unlock()
		}
	}
}

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrBusy means another Manager (possibly in another process) owns the session.
var ErrBusy = errors.New("session: already active in another manager; close it before resuming")

// Close releases writable ownership. It is idempotent and does not flush buffered
// user-only conversations. Reads remain valid; subsequent writes return os.ErrClosed.
func (sm *Manager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return sm.closeErr
	}
	sm.closed = true
	if sm.owner != nil {
		sm.closeErr = sm.owner.Close() // closing the handle releases the kernel lock
		sm.owner = nil
	}
	return sm.closeErr
}

// Resolve aliases before choosing the sidecar and before writing the JSONL. A new
// session has no JSONL yet, so resolve its existing parent directory instead.
func canonicalSessionPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("session: absolute path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("session: resolve %s: %w", path, err)
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("session: resolve directory of %s: %w", path, err)
	}
	return filepath.Join(dir, filepath.Base(abs)), nil
}

func ownershipPath(path string) string {
	return filepath.Join(filepath.Dir(path), ".locks", filepath.Base(path)+".lock")
}

// Sidecars are permanent: unlinking one could let a new opener lock a different
// inode while the original owner still holds the old one. JSONL rename is safe.
func acquireOwnership(path string) (*os.File, error) {
	lockPath := ownershipPath(path)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("session: create ownership directory: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("session: open ownership lock: %w", err)
	}
	// A probe holds its shared lock for microseconds. Retrying briefly means
	// only a real owner, not a listing that happened to overlap, reports busy.
	lockErr := tryOwnershipLock(f)
	for delay := time.Millisecond; errors.Is(lockErr, ErrBusy) && delay <= 8*time.Millisecond; delay *= 2 {
		time.Sleep(delay)
		lockErr = tryOwnershipLock(f)
	}
	if lockErr != nil {
		return nil, errors.Join(fmt.Errorf("session: own %s: %w", path, lockErr), f.Close())
	}
	return f, nil
}

// Probing never creates a directory or file and never touches JSONL contents.
// Active is advisory; only acquisition can decide who owns a session. The probe
// takes a shared lock, so concurrent probes never report each other as owners.
func probeOwnership(path string) (bool, error) {
	path, err := canonicalSessionPath(path)
	if err != nil {
		return false, err
	}
	f, err := os.Open(ownershipPath(path))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("session: probe ownership: %w", err)
	}
	lockErr := tryProbeLock(f)
	closeErr := f.Close()
	if errors.Is(lockErr, ErrBusy) {
		return true, closeErr
	}
	return false, errors.Join(lockErr, closeErr)
}

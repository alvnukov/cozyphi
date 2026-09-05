package atomicfile

import (
	"path/filepath"
	"sync"
)

// lockEntry is one path's turnstile. refs counts the writer holding it plus
// every writer waiting for it, so the registry can drop the entry the moment
// the last one leaves and never grows with the paths a session touches.
type lockEntry struct {
	mu   sync.Mutex
	refs int
}

// lockRegistry hands out one mutex per destination path. Its own mutex is held
// only for the map lookup, never while a writer waits.
type lockRegistry struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

// writeLocks serializes the cooperating writers of one path — every caller of
// this package in this process, whatever session or job they belong to. The
// key is lexical (see lockKey), so two aliases of one file through different
// directory symlinks are not serialized with each other; to one another they
// are arbitrary external writers, which the package doc describes.
var writeLocks = lockRegistry{entries: make(map[string]*lockEntry)}

// lockPath blocks until this process's writers of path are all behind the
// caller and returns the release. Callers hold it across every pre-rename
// check and the rename itself, so no cooperating writer can slip between a
// check and the swap it authorizes.
func lockPath(path string) func() {
	key := lockKey(path)
	writeLocks.mu.Lock()
	entry := writeLocks.entries[key]
	if entry == nil {
		entry = &lockEntry{}
		writeLocks.entries[key] = entry
	}
	entry.refs++
	writeLocks.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		writeLocks.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(writeLocks.entries, key)
		}
		writeLocks.mu.Unlock()
	}
}

// lockKey names the path the way two callers spelling it differently — one
// relative, one absolute, one with a stray "." — still meet on the same
// entry. Resolution stops at the lexical form: EvalSymlinks would take the
// filesystem's word for a path the caller has not yet re-verified, and a
// missing file has no resolved name at all.
func lockKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

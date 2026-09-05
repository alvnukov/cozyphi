package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForQueuedWriter spins until a second contender has joined path's lock.
// Joining is the last thing a writer does before waiting, and it happens after
// staging, so the caller knows exactly where the queued writer stands: bytes
// on disk, not one pre-rename check made. Spinning beats sleeping — the
// interleaving is exact instead of probable.
func waitForQueuedWriter(path string) {
	key := lockKey(path)
	for {
		writeLocks.mu.Lock()
		entry := writeLocks.entries[key]
		queued := entry != nil && entry.refs > 1
		writeLocks.mu.Unlock()
		if queued {
			return
		}
		runtime.Gosched()
	}
}

// The bug this ordering exists for: a writer that lands on the target after
// the last guard ran used to be overwritten by the rename, and the caller was
// told the write succeeded. Verify runs last now, so the late writer's bytes
// are the ones it judges — and it refuses.
func TestVerifyCatchesWriterLandingAfterTheGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	rejected := errors.New("file changed during edit")
	calls := 0
	err := WriteWith(path, 0o644, []byte("payload"), Options{
		Guard: func(string) error {
			calls++
			if calls < 2 {
				return nil
			}
			// The second call is the last check before the swap: a writer
			// landing here used to be invisible.
			return os.WriteFile(path, []byte("someone else got here first"), 0o644)
		},
		Verify: func(current []byte) error {
			if string(current) != "original" {
				return rejected
			}
			return nil
		},
	})

	require.ErrorIs(t, err, rejected)
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "someone else got here first", string(got), "the writer that landed last must survive")
	entries, listErr := os.ReadDir(dir)
	require.NoError(t, listErr)
	assert.Len(t, entries, 1, "the abandoned staging file is removed")
}

// Cooperating writers — any two callers of this package in one process — take
// the path in turns. The second writer's Verify must not run until the first
// has renamed, and it must see the first writer's bytes: a read-modify-write
// cycle cannot lose an update it never saw.
func TestCooperatingWritersSerializePerPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	verifying := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var firstErr, secondErr error
	var secondSaw []byte

	wg.Go(func() {
		firstErr = WriteChecked(path, 0o644, []byte("first writer"), func(_ []byte) error {
			close(verifying)
			<-release
			return nil
		})
	})
	<-verifying

	wg.Go(func() {
		secondErr = WriteChecked(path, 0o644, []byte("second writer"), func(current []byte) error {
			secondSaw = append([]byte(nil), current...)
			return nil
		})
	})
	// The second writer has staged its bytes and is parked on the lock: its
	// Verify cannot have run, because Verify runs inside the lock.
	waitForQueuedWriter(path)
	assert.Nil(t, secondSaw, "the second writer verified while the first still held the path")

	close(release)
	wg.Wait()

	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, "first writer", string(secondSaw), "the queued writer verifies against the landed revision")
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, "second writer", string(got))
}

// The lock is per path, not a queue for the whole process: a writer blocked on
// one file leaves every other file writable. Were it global, this test would
// deadlock rather than fail.
func TestWritersOnDifferentPathsDoNotBlockEachOther(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked.txt")
	other := filepath.Join(dir, "other.txt")
	require.NoError(t, os.WriteFile(blocked, []byte("original"), 0o644))
	require.NoError(t, os.WriteFile(other, []byte("original"), 0o644))

	verifying := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var blockedErr error
	wg.Go(func() {
		blockedErr = WriteChecked(blocked, 0o644, []byte("payload"), func(_ []byte) error {
			close(verifying)
			<-release
			return nil
		})
	})
	<-verifying

	verified := false
	err := WriteChecked(other, 0o644, []byte("payload"), func(_ []byte) error {
		verified = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, verified, "the second path verified and swapped while the first was held")

	close(release)
	wg.Wait()
	require.NoError(t, blockedErr)
}

// The registry holds nothing once the writers leave: a session that touches
// thousands of files must not grow a mutex per file.
func TestPathLockRegistryDropsIdleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")

	require.NoError(t, Write(path, 0o644, []byte("payload")))

	writeLocks.mu.Lock()
	remaining := len(writeLocks.entries)
	writeLocks.mu.Unlock()
	assert.Zero(t, remaining, "a finished write left its lock entry behind")
}

func TestLockKeyNormalizesSpelling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")

	assert.Equal(t, lockKey(path), lockKey(filepath.Join(dir, ".", "note.txt")))
	assert.Equal(t, lockKey(path), lockKey(filepath.Join(dir, "sub", "..", "note.txt")))
	assert.True(t, filepath.IsAbs(lockKey("note.txt")), "a relative path keys on its absolute form")
}

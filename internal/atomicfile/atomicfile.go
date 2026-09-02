// Package atomicfile replaces file contents so a reader never observes a torn
// write: data lands in a same-directory temporary file, which is synced and
// renamed over the target in one step. Any failure before the rename leaves
// the previous contents untouched, and the staging file is removed.
//
// The replacement never writes through a symlink. rename replaces the path
// itself rather than what it points at, a leaf that is a symlink at mutation
// time fails closed instead of clobbering it, and the staging directory is
// resolved once and re-verified immediately before the rename so a symlink
// swapped into an ancestor mid-write aborts the replacement.
//
// Re-verification alone cannot see an ancestor swapped before the write
// began: the module resolves what the path points at now, not what the
// permission check saw. A caller that has a rule about where the write may
// land passes a Guard, which is asked to judge the path twice — before any
// directory is created and again immediately before the rename — so a
// redirected ancestor fails closed instead of landing the file wherever the
// link leads. Without a Guard the module still refuses to follow a leaf
// symlink, and still aborts on an ancestor swapped during the write, but a
// swap performed before the call is invisible to it.
//
// The residual window — the two syscalls between the last check and the
// rename — is the check-then-act floor without descriptor-relative opens;
// inside it the leaf is still safe because a rename never follows a symlink.
// Cleanup of the staging file is best-effort: an ancestor directory renamed
// mid-write can strand its dotfile beside the original.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Options carry the optional guards of a replacement. The zero value is a
// plain atomic write.
type Options struct {
	// Verify is the last-chance guard for a read-modify-write cycle:
	// immediately before the staged file replaces the target, it receives the
	// target's current bytes (read without following a leaf symlink), and a
	// non-nil error abandons the swap with the target untouched.
	Verify func(current []byte) error

	// Guard judges the destination path itself. It runs before the parent
	// directories are created and again immediately before the rename; a
	// non-nil error abandons the write. Callers whose permission check
	// resolved the path earlier use it to re-apply that verdict, so a
	// directory swapped for a symlink after the check cannot redirect the
	// file.
	Guard func(path string) error
}

// Write replaces path with data. mode is the file's final permission set,
// applied explicitly so a restrictive umask cannot surprise the caller. A
// missing parent directory is created.
func Write(path string, mode os.FileMode, data []byte) error {
	return write(path, mode, data, Options{})
}

// WriteChecked is Write with Options.Verify.
func WriteChecked(path string, mode os.FileMode, data []byte, verify func(current []byte) error) error {
	return write(path, mode, data, Options{Verify: verify})
}

// WriteWith is Write under opts.
func WriteWith(path string, mode os.FileMode, data []byte, opts Options) error {
	return write(path, mode, data, opts)
}

// ReadNoFollow reads path without ever following a leaf symlink: a path that
// was a regular file at permission-check time but a symlink by read time
// yields a fail-closed error instead of the target's bytes. Read-modify-write
// callers use it so foreign content cannot flow into diffs, guards or error
// messages.
func ReadNoFollow(path string) ([]byte, error) {
	file, err := openNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	return io.ReadAll(file)
}

func write(path string, mode os.FileMode, data []byte, opts Options) (retErr error) {
	// Asked before MkdirAll: creating the parents of a redirected path would
	// already plant directories wherever the swapped ancestor leads.
	if err := guard(opts, path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	// Stage in the resolved directory, not the lexical one: the staging path
	// then contains no symlinks itself, so a later ancestor swap cannot
	// redirect the deferred cleanup, and the re-verification below compares
	// the target's directory against this physical anchor.
	stageDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve directory %s: %w", dir, err)
	}
	file, err := os.CreateTemp(stageDir, tempPattern(path))
	if err != nil {
		return fmt.Errorf("stage replacement for %s: %w", path, err)
	}
	tmp := file.Name()
	closed, renamed := false, false
	defer func() {
		if !closed {
			if closeErr := file.Close(); retErr == nil && closeErr != nil {
				retErr = fmt.Errorf("close staging file %s: %w", tmp, closeErr)
			}
		}
		if !renamed {
			if removeErr := os.Remove(tmp); retErr == nil && removeErr != nil &&
				!errors.Is(removeErr, os.ErrNotExist) {
				retErr = fmt.Errorf("remove staging file %s: %w", tmp, removeErr)
			}
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set permissions %o on staging file %s: %w", mode, tmp, err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write staging file %s: %w", tmp, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync staging file %s: %w", tmp, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staging file %s: %w", tmp, err)
	}
	closed = true
	if opts.Verify != nil {
		current, err := ReadNoFollow(path)
		if err != nil {
			return fmt.Errorf("re-read %s before replacing: %w", path, err)
		}
		if err := opts.Verify(current); err != nil {
			return err
		}
	}
	// A leaf symlink is refused rather than replaced: overwriting it would
	// destroy an alias the caller never named, and following it could not be
	// safe at all.
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("replace %s: path is a symlink, refusing to overwrite or follow it", path)
	}
	if now, err := filepath.EvalSymlinks(dir); err != nil || now != stageDir {
		return fmt.Errorf("replace %s: directory %s changed during the write", path, dir)
	}
	// Asked again with the staging file already on disk: an ancestor swapped
	// while the bytes were being written is caught here, and the staged file
	// is removed by the deferred cleanup.
	if err := guard(opts, path); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	renamed = true
	return nil
}

func guard(opts Options, path string) error {
	if opts.Guard == nil {
		return nil
	}
	if err := opts.Guard(path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// tempPattern hides the staging file from globbers that skip dotfiles while
// naming it after the file it replaces.
func tempPattern(path string) string {
	return "." + filepath.Base(path) + "-*.tmp"
}

// Package atomicfile replaces file contents so a reader never observes a torn
// write: data lands in a same-directory temporary file, which is synced and
// renamed over the target in one step. Any failure before the rename leaves
// the previous contents untouched, and the staging file is removed.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces path with data. mode is the file's final permission set,
// applied explicitly so a restrictive umask cannot surprise the caller. A
// missing parent directory is created.
func Write(path string, mode os.FileMode, data []byte) error {
	return write(path, mode, data, nil)
}

// WriteChecked is Write with a last-chance guard for a read-modify-write
// cycle: immediately before the staged file replaces the target, verify
// receives the target's current bytes, and a non-nil error abandons the
// swap with the target untouched. It shrinks the check-to-write window to
// the two syscalls between the re-read and the rename.
func WriteChecked(path string, mode os.FileMode, data []byte, verify func(current []byte) error) error {
	return write(path, mode, data, verify)
}

func write(path string, mode os.FileMode, data []byte, verify func(current []byte) error) (retErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	file, err := os.CreateTemp(dir, tempPattern(path))
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
	if verify != nil {
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("re-read %s before replacing: %w", path, err)
		}
		if err := verify(current); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	renamed = true
	return nil
}

// tempPattern hides the staging file from globbers that skip dotfiles while
// naming it after the file it replaces.
func tempPattern(path string) string {
	return "." + filepath.Base(path) + "-*.tmp"
}

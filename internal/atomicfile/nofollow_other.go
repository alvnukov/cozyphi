//go:build !darwin && !linux

package atomicfile

import "os"

// openNoFollow is the best-effort fallback for platforms without O_NOFOLLOW
// in the standard library. The mutation path still refuses leaf symlinks via
// Lstat immediately before the rename, so staging never writes through one;
// only this guard read can follow a link here.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

//go:build !unix

package atomicfile

import "os"

// openNoFollow is the best-effort fallback for the platforms with no
// O_NOFOLLOW at all — Windows, wasm — since every unix builds the real one.
// The mutation path still refuses leaf symlinks via Lstat immediately before
// the rename, so staging never writes through one; only this guard read can
// follow a link here.
func openNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}

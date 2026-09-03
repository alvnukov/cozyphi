package writetool

import "os"

// defaultFileMode is what a file gets when there is nothing to preserve: a
// fresh file, or a destination the replacement will refuse anyway.
const defaultFileMode = os.FileMode(0o644)

// destinationMode is the permission set a replacement of path must land with.
// The write and edit tools rewrite content, not permissions, so an existing
// regular file keeps the mode it already has: replacing a 0755 script must
// not cost it its exec bit, and the staged file is renamed over the target,
// so nothing else would carry the old mode across.
//
// Lstat reads the path itself rather than what a link points at, so a symlink
// swapped in after the permission check cannot donate its own permissions to
// the replacement; anything that is not a regular file falls back to the
// default and is refused inside the mutation module.
func destinationMode(path string) os.FileMode {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return defaultFileMode
	}
	return info.Mode().Perm()
}

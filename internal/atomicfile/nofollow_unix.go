//go:build unix

package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// openNoFollow opens path refusing to resolve a leaf symlink: ELOOP surfaces
// as a fail-closed error naming the path, so a swapped link never turns into
// a read of whatever it points at.
func openNoFollow(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == syscall.ELOOP {
			return nil, fmt.Errorf("open %s: path is a symlink, refusing to follow", path)
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return file, nil
}

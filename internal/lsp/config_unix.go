//go:build !windows

package lsp

import (
	"os"
	"syscall"
)

// openConfigFile opens the config without following a final symlink, closing
// the Lstat-to-open race where the path could be swapped for a link.
func openConfigFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

// configOwnedByCurrentUser reports whether the open file belongs to the
// running user. Foreign-owned configs are an escalation vector: another
// account could plant a server command.
func configOwnedByCurrentUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		// No stat metadata means we cannot prove ownership: fail closed.
		return false
	}
	// UIDs compare exactly: widening both sides keeps the check portable
	// across 32- and 64-bit builds without any narrowing conversion. getuid
	// is never negative, so the signed compare cannot alias two owners.
	return int64(st.Uid) == int64(os.Getuid())
}

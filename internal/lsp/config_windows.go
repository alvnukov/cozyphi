//go:build windows

package lsp

import "os"

// openConfigFile opens the config file. Windows has no O_NOFOLLOW; the Lstat
// symlink check in LoadConfig plus the post-open regular-file and mode checks
// cover the same invariant where the platform supports it.
func openConfigFile(path string) (*os.File, error) {
	return os.Open(path)
}

// configOwnedByCurrentUser is not enforceable portably on Windows: file
// ownership is ACL-based and the Go FileInfo carries no owner. The mode and
// regular-file checks still apply; per-user profile directories provide the
// practical isolation.
func configOwnedByCurrentUser(fi os.FileInfo) bool {
	return true
}

// configWorldOrGroupWritable is a no-op on Windows. NTFS access is ACL-based
// and the Go FileInfo reports a synthetic 0666 mode for every file, so the
// unix group/world-writable test would reject every config and no LSP config
// could ever load. Per-user profile directories provide the isolation instead.
func configWorldOrGroupWritable(fi os.FileInfo) bool {
	return false
}

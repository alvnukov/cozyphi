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

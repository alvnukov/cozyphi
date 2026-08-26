//go:build !windows

package lsp

import "os"

// isExecutable reports whether path is a regular file with an execute bit.
// The path always originates from the owner's config or a sanitized PATH
// entry, never from model input.
func isExecutable(path string) bool {
	st, err := os.Stat(path) //nolint:gosec // G703: owner-controlled path, no model input
	return err == nil && !st.IsDir() && st.Mode().Perm()&0o111 != 0
}

//go:build windows

package lsp

import "os"

// isExecutable approximates executability on Windows, where the execute bit
// does not exist: PATHEXT-style extensions are accepted as-is.
func isExecutable(path string) bool {
	for _, candidate := range []string{path, path + ".exe", path + ".bat", path + ".cmd"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

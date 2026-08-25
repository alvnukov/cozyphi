package lsp

import (
	"fmt"
	"path/filepath"
	"strings"
)

// contained reports whether absPath physically lives inside the canonical
// workspace. Both tool input and every returned file URI pass through this
// seam before entering Result. Symlinks and ancestor symlinks are resolved, so
// a lexical path that escapes through a link fails closed.
func contained(workspace, absPath string) (bool, error) {
	wsCanon, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return false, fmt.Errorf("lsp: resolve workspace: %w", err)
	}
	canon, err := canonicalizeNearest(absPath)
	if err != nil {
		return false, fmt.Errorf("lsp: resolve path: %w", err)
	}
	rel, err := filepath.Rel(wsCanon, canon)
	if err != nil {
		return false, nil
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// canonicalizeNearest resolves symlinks in the deepest existing ancestor of
// path and rejoins the non-existing remainder, so not-yet-created files are
// still checked against the physical target of their parent chain.
func canonicalizeNearest(path string) (string, error) {
	path = filepath.Clean(path)
	var suffix []string
	cur := path
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			if len(suffix) == 0 {
				return resolved, nil
			}
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve any ancestor of %s", path)
		}
		suffix = append([]string{filepath.Base(cur)}, suffix...)
		cur = parent
	}
}

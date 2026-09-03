package permission

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveTarget returns the physical filesystem path that path points at.
// Every symlink in the deepest existing ancestor chain is resolved; tail
// components that do not exist yet cannot point anywhere and are appended
// unchanged. Only absolute paths are accepted: relative inputs would be
// resolved against an implicit cwd the caller never agreed to.
//
// Resolution errors (permission denied on a component, symlink loop) fail
// closed as errors — the caller must treat the path as unverifiable.
func ResolveTarget(path string) (string, error) {
	if path == "" {
		return "", errors.New("resolve target: empty path")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolve target: %q is not an absolute path", path)
	}
	clean := filepath.Clean(path)
	current := clean
	var missing []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, rerr := filepath.EvalSymlinks(current)
			if rerr != nil {
				return "", fmt.Errorf("resolve target %q: %w", path, rerr)
			}
			if len(missing) == 0 {
				return resolved, nil
			}
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve target %q: %w", path, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve target %q: no existing ancestor", path)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}
}

// WithinWorkspaceResolved reports whether path stays inside workspace once
// symlinks are resolved on both sides. It fails closed: any resolution error
// counts as outside, so callers cannot mistake an unverifiable workdir for a
// contained one. Use this wherever a workdir or spawn boundary is accepted.
func WithinWorkspaceResolved(path, workspace string) bool {
	resolvedPath, err := ResolveTarget(path)
	if err != nil {
		return false
	}
	resolvedWorkspace, err := ResolveTarget(workspace)
	if err != nil {
		return false
	}
	return InWorkspace(resolvedPath, resolvedWorkspace)
}

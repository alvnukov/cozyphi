package pathutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ShortPath abbreviates a filesystem path for display.
func ShortPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) <= 5 {
		return p
	}
	return strings.Join(parts[:2], string(filepath.Separator)) +
		string(filepath.Separator) + ".." + string(filepath.Separator) +
		strings.Join(parts[len(parts)-2:], string(filepath.Separator))
}

// GitBranch returns the current git branch of dir, or "" when unavailable.
func GitBranch(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PathWithBranch renders the short path plus the git branch, e.g. "~/repo (main)".
func PathWithBranch(dir string) string {
	label := ShortPath(dir)
	if branch := GitBranch(context.Background(), dir); branch != "" {
		label += " (" + branch + ")"
	}
	return label
}

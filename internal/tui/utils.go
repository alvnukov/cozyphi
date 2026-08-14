package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func shortPath(p string) string {
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

// gitBranch returns the current git branch of dir, or "" when dir is not
// inside a git repository (including detached HEAD).
func gitBranch(ctx context.Context, dir string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pathWithBranch renders the short path plus the git branch, e.g. "~/repo (main)".
func pathWithBranch(dir string) string {
	label := shortPath(dir)
	// gitBranch is a short-lived, synchronous lookup; its callers have no ctx.
	if branch := gitBranch(context.Background(), dir); branch != "" {
		label += " (" + branch + ")"
	}
	return label
}

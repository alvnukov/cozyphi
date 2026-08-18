package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// branchPollInterval is how often the TUI checks the repo HEAD for a branch
// switch. Reading a one-line file is free; the git process only runs when
// HEAD actually moved.
const branchPollInterval = time.Second

// branchWatch hot-reloads the git branch shown in the path label. It polls
// the repo HEAD so checkouts made outside the TUI (another terminal, an
// editor) show up without a restart.
type branchWatch struct {
	dir      string
	interval time.Duration
}

// run polls until stop is closed, calling publish with the fresh label
// whenever the checked-out branch changes. publish may be called from this
// goroutine only; the caller must not block on it.
func (b *branchWatch) run(stop <-chan struct{}, publish func(label string)) {
	if b.interval <= 0 {
		b.interval = branchPollInterval
	}
	last := branchState(b.dir)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		if cur := branchState(b.dir); cur != last {
			last = cur
			publish(pathWithBranch(b.dir))
		}
	}
}

// branchState is a digest of the checked-out branch: HEAD's content, or
// "missing" when dir is not inside a git repository. Commits and ref
// updates do not touch HEAD, so they never republish the label.
func branchState(dir string) string {
	gitDir := resolveGitDir(dir)
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(string(data))
}

// resolveGitDir returns the git directory for dir: dir/.git, or the target
// of a worktree/submodule .git file ("gitdir: <path>").
func resolveGitDir(dir string) string {
	dotGit := filepath.Join(dir, ".git")
	if data, err := os.ReadFile(dotGit); err == nil {
		if target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
			target = strings.TrimSpace(target)
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			return target
		}
	}
	return dotGit
}

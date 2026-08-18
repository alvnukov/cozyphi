package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const branchTestInterval = 20 * time.Millisecond

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	require.NoError(t, cmd.Run(), "git %v", args)
}

func TestResolveGitDir(t *testing.T) {
	dir := t.TempDir()
	// Plain repository: dir/.git is a directory.
	assert.Equal(t, filepath.Join(dir, ".git"), resolveGitDir(dir))

	// Worktree/submodule: .git is a file pointing at the real git dir,
	// either relative to dir or absolute.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../modules/sub\n"), 0o644))
	assert.Equal(t, filepath.Join(filepath.Dir(dir), "modules", "sub"), resolveGitDir(dir))

	abs := filepath.Join(t.TempDir(), "modules", "sub")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+abs+"\n"), 0o644))
	assert.Equal(t, abs, resolveGitDir(dir))
}

func TestBranchWatchPublishesOnSwitch(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")

	published := make(chan string, 4)
	stop := make(chan struct{})
	defer close(stop)
	go (&branchWatch{dir: repo, interval: branchTestInterval}).run(stop, func(label string) {
		published <- label
	})

	// Commit noise on the same branch must not republish.
	git(t, repo, "commit", "--allow-empty", "-q", "-m", "noise")
	git(t, repo, "checkout", "-q", "-b", "feature")
	select {
	case label := <-published:
		assert.Contains(t, label, " (feature)")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a publish after switching to feature")
	}

	git(t, repo, "checkout", "-q", "main")
	select {
	case label := <-published:
		assert.Contains(t, label, " (main)")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a publish after switching back to main")
	}
}

func TestBranchWatchDetachesToPlainPath(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "commit", "--allow-empty", "-q", "-m", "init")

	published := make(chan string, 4)
	stop := make(chan struct{})
	defer close(stop)
	go (&branchWatch{dir: repo, interval: branchTestInterval}).run(stop, func(label string) {
		published <- label
	})

	// Detached HEAD has no branch: the label loses its suffix.
	git(t, repo, "checkout", "-q", "--detach")
	select {
	case label := <-published:
		assert.Equal(t, shortPath(repo), label)
	case <-time.After(2 * time.Second):
		t.Fatal("expected a publish after detaching HEAD")
	}
}

func TestBranchWatchDetectsLateInit(t *testing.T) {
	dir := t.TempDir()
	published := make(chan string, 4)
	stop := make(chan struct{})
	defer close(stop)
	go (&branchWatch{dir: dir, interval: branchTestInterval}).run(stop, func(label string) {
		published <- label
	})

	// dir starts outside git; initializing a repo must start the suffix.
	git(t, dir, "init", "-q", "-b", "main")
	select {
	case label := <-published:
		assert.Contains(t, label, " (main)")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a publish after git init")
	}
}

func TestBranchWatchSilentWithoutChange(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")

	published := make(chan string, 4)
	stop := make(chan struct{})
	defer close(stop)
	go (&branchWatch{dir: repo, interval: 10 * time.Millisecond}).run(stop, func(label string) {
		published <- label
	})

	select {
	case label := <-published:
		t.Fatalf("unexpected publish without a branch change: %s", label)
	case <-time.After(5 * branchTestInterval):
	}
}

func TestEditorAppliesBranchLabel(t *testing.T) {
	editor := &Editor{}
	editor.Chat.BottomRightLabel.Text = "~ (old)"
	editor.Update(BranchLabelMsg{Text: "~ (new)"})
	assert.Equal(t, "~ (new)", editor.Chat.BottomRightLabel.Text)
}

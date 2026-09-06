package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitRepo initializes a repository with one commit and returns it, skipping
// when git is unavailable.
func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).Run(); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	cmd := exec.CommandContext(t.Context(), "git", "-C", repo,
		"commit", "--quiet", "--allow-empty", "-m", "init")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit: %s", out)
	return repo
}

// The anchors are the layout's own arithmetic, so they are answerable
// wherever the workspace sits — and they are the parts of it that a locator
// may be composed from.
func TestStoreAnchorsAreTheLayoutsOwnDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	proj, err := Discover(t.TempDir())
	require.NoError(t, err)

	anchors := proj.StoreAnchors()

	assert.True(t, anchors.Known)
	assert.Equal(t, proj.Global().SessionBase(), anchors.SessionBase)
	assert.Equal(t, proj.Global().claudeProjectsDir(), anchors.MemoryBase)
	assert.Equal(t, proj.Global().Root(), anchors.UsageDir)
	assert.Equal(t, proj.Global().UsageFile(), anchors.UsageFile)
	assert.Equal(t, filepath.Dir(anchors.UsageFile), anchors.UsageDir,
		"the file is in the directory reported beside it")
}

// Two worktrees of one checkout share memories and do not share transcripts,
// and nothing else about a session says so. These two flags are where that
// is decided, and outside Git there is no checkout to share with.
func TestTheCorpusFlagsSeparateAWorktreeFromACheckoutAndFromNeither(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := gitRepo(t)
	worktree := filepath.Join(repo, ".worktrees", "topic")
	add := exec.CommandContext(t.Context(), "git", "-C", repo,
		"worktree", "add", "--quiet", "-b", "topic", worktree)
	out, err := add.CombinedOutput()
	require.NoError(t, err, "git worktree add: %s", out)
	subdir := filepath.Join(repo, "internal", "diag")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	for _, tc := range []struct {
		name    string
		dir     string
		shared  bool
		foreign bool
	}{
		{name: "the checkout itself", dir: repo, shared: true},
		{name: "a directory inside it", dir: subdir, shared: true, foreign: true},
		{name: "a linked worktree", dir: worktree, shared: true, foreign: true},
		{name: "no repository at all", dir: t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj, err := Discover(tc.dir)
			require.NoError(t, err)
			anchors := proj.StoreAnchors()
			assert.Equal(t, tc.shared, anchors.CorpusShared)
			assert.Equal(t, tc.foreign, anchors.CorpusForeign)
		})
	}
}

// Asking where a store would be must not bring it into existence: the memory
// corpus is created on open, and a view that stat'd or made these
// directories would answer a question by changing the answer.
func TestAskingWhereTheStoresGoCreatesNoneOfThem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	proj, err := Discover(t.TempDir())
	require.NoError(t, err)
	before, err := os.ReadDir(home)
	require.NoError(t, err)

	first := proj.StoreAnchors()
	for range 3 {
		assert.Equal(t, first, proj.StoreAnchors())
	}

	after, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Len(t, after, len(before))
	_, statErr := os.Stat(proj.MemoryDir())
	assert.True(t, os.IsNotExist(statErr), "the corpus directory is made by opening one, not by asking")
	_, statErr = os.Stat(first.UsageFile)
	assert.True(t, os.IsNotExist(statErr))
}

// A project nobody resolved is a wiring gap. The layer above turns that into
// "the layout is not known", so no locator is composed from an empty anchor.
func TestNoProjectPublishesNoLayout(t *testing.T) {
	var missing *Project
	assert.False(t, missing.StoreAnchors().Known)
}

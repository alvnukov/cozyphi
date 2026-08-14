package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitBranch(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, gitBranch(t.Context(), dir), "non-git dir has no branch")

	require.NoError(t, exec.CommandContext(t.Context(), "git", "-C", dir, "init", "-q", "-b", "main").Run())
	assert.Equal(t, "main", gitBranch(t.Context(), dir))

	require.NoError(t, exec.CommandContext(t.Context(), "git", "-C", dir, "checkout", "-q", "-b", "feat/ui").Run())
	assert.Equal(t, "feat/ui", gitBranch(t.Context(), dir))
}

func TestPathWithBranch(t *testing.T) {
	plain := t.TempDir()
	assert.Equal(t, shortPath(plain), pathWithBranch(plain), "no branch suffix outside git")

	repo := filepath.Join(plain, "repo")
	require.NoError(t, os.Mkdir(repo, 0o755))
	require.NoError(t, exec.CommandContext(t.Context(), "git", "-C", repo, "init", "-q", "-b", "main").Run())
	assert.Equal(t, shortPath(repo)+" (main)", pathWithBranch(repo))
}

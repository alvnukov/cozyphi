package tasks_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

func withRegistry(t *testing.T, root string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, tasks.DefaultDir), 0o755))
	return root
}

// resolved is a path in the spelling targets canonicalize to (symlinks
// resolved), which on some machines differs from the one t.TempDir returns.
func resolved(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return out
}

// The default is the launch checkout's own registry: a session in a worktree
// works that worktree's ledger, and its label is the name a write says back.
func TestDiscoverTargetsDefaultsToTheLaunchCheckout(t *testing.T) {
	main, launch := withRegistry(t, t.TempDir()), withRegistry(t, t.TempDir())

	targets, err := tasks.DiscoverTargets(launch, main, nil)
	require.NoError(t, err)
	require.NotNil(t, targets)

	assert.Equal(t, resolved(t, launch), targets.Default().Root())
	assert.Equal(t, filepath.Base(launch), targets.DefaultLabel())

	reg, err := targets.Resolve("")
	require.NoError(t, err)
	assert.Equal(t, resolved(t, launch), reg.Root())
}

// A launch checkout without a registry keeps the old guarantee another way:
// the main checkout's registry answers, so a worktree on a branch that
// predates the registry directory still sees one.
func TestDiscoverTargetsFallsBackToTheMainCheckout(t *testing.T) {
	main, launch := withRegistry(t, t.TempDir()), t.TempDir()

	targets, err := tasks.DiscoverTargets(launch, main, nil)
	require.NoError(t, err)
	require.NotNil(t, targets)

	assert.Equal(t, resolved(t, main), targets.Default().Root())
	assert.Equal(t, "main", targets.DefaultLabel())
}

func TestDiscoverTargetsSurfacesLaunchConfigErrorsAndAbsence(t *testing.T) {
	broken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(broken, tasks.ConfigFile), []byte("task_registry: ["), 0o644))

	if _, err := tasks.DiscoverTargets(broken, "", nil); assert.Error(t, err) {
		assert.Contains(t, err.Error(), tasks.ConfigFile)
	}

	targets, err := tasks.DiscoverTargets(t.TempDir(), t.TempDir(), nil)
	require.NoError(t, err)
	assert.Nil(t, targets, "no registry anywhere: no targets, and the tool stays out")
}

// Snapshot lists what a call may address: the default first, configured
// external roots by their names, and Git's live worktrees by theirs. A root
// without a registry is not a target, and an unknown label names the ones
// that are.
func TestSnapshotListsVouchedRootsAndRefusesTheRest(t *testing.T) {
	main := withRegistry(t, t.TempDir())
	external := withRegistry(t, t.TempDir())
	absent := t.TempDir()

	targets, err := tasks.DiscoverTargets(main, main, map[string]string{"docs": external, "gone": absent})
	require.NoError(t, err)

	snap := targets.Snapshot()
	require.Len(t, snap, 2)
	assert.Equal(t, "main", snap[0].Label)
	assert.Equal(t, "docs", snap[1].Label)

	_, err = targets.Resolve("gone")
	require.Error(t, err)
	assert.ErrorContains(t, err, `unknown registry root "gone" (known: main, docs)`)

	reg, err := targets.Resolve("docs")
	require.NoError(t, err)
	assert.Equal(t, resolved(t, external), reg.Root())
}

// A worktree made after the session started is addressable on the next call:
// the worktree list is Git's answer at resolve time, not a startup snapshot.
func TestSnapshotSeesWorktreesBornMidSession(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	if err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).Run(); err != nil {
		t.Skipf("git is unavailable: %v", err)
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repo, tasks.DefaultDir), 0o755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(repo, tasks.DefaultDir, "seed.md"),
			[]byte("---\nid: seed\ntitle: Seed\n---\n"),
			0o644,
		),
	)
	git("add", ".")
	git("commit", "--quiet", "-m", "seed")

	targets, err := tasks.DiscoverTargets(repo, repo, nil)
	require.NoError(t, err)
	require.NotNil(t, targets)
	require.Len(t, targets.Snapshot(), 1, "only the main checkout so far")

	worktree := filepath.Join(repo, ".worktrees", "fix-login")
	git("worktree", "add", "--quiet", "-b", "fix-login", worktree)

	snap := targets.Snapshot()
	require.Len(t, snap, 2)
	assert.Equal(t, "main", snap[0].Label)
	assert.Equal(t, "fix-login", snap[1].Label)

	reg, err := targets.Resolve("fix-login")
	require.NoError(t, err)
	assert.Equal(t, resolved(t, worktree), reg.Root())
}

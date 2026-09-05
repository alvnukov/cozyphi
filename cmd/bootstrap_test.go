package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
)

// testProject discovers a project under a temp HOME so tests never touch the
// real ~/.cozyphi, and returns a project plus a PATH dir for binary stubs.
func testProject(t *testing.T) (*project.Project, string) {
	t.Helper()
	home := t.TempDir()
	pathDir := t.TempDir()
	// os.UserHomeDir uses HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", pathDir)

	p, err := project.Discover("")
	require.NoError(t, err)
	return p, pathDir
}

func TestShouldBootstrapWhenMissing(t *testing.T) {
	p, _ := testProject(t)
	// Empty bin dir and empty PATH dir → must download.
	assert.True(t, shouldBootstrap(p, "fd"))
	assert.True(t, shouldBootstrap(p, "rg"))
}

func TestShouldBootstrapWhenInBinDir(t *testing.T) {
	p, _ := testProject(t)
	binName := "fd"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(p.Global().BinDir(), binName), []byte("x"), 0o755))

	assert.False(t, shouldBootstrap(p, "fd"))
	assert.True(t, shouldBootstrap(p, "rg"))
}

func TestShouldBootstrapWhenOnPATH(t *testing.T) {
	p, pathDir := testProject(t)
	binName := "rg"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(pathDir, binName), []byte("x"), 0o755))

	assert.False(t, shouldBootstrap(p, "rg"))
	assert.True(t, shouldBootstrap(p, "fd"))
}

func TestEnsureSearchToolsAttemptsAllDownloads(t *testing.T) {
	p, _ := testProject(t)
	started := make(chan string, 2)
	release := make(chan struct{})
	download := func(_ context.Context, tool string) (string, error) {
		started <- tool
		<-release
		return "", errors.New("download failed")
	}

	result := make(chan error, 1)
	go func() {
		result <- ensureSearchTools(t.Context(), p, download)
	}()

	downloaded := make([]string, 0, 2)
	for len(downloaded) < 2 {
		select {
		case tool := <-started:
			downloaded = append(downloaded, tool)
		case <-time.After(time.Second):
			close(release)
			<-result
			t.Fatal("downloads did not start concurrently")
		}
	}
	close(release)
	err := <-result
	require.Error(t, err)
	assert.ElementsMatch(t, []string{"fd", "rg"}, downloaded)
	assert.ErrorContains(t, err, "fd: download failed")
	assert.ErrorContains(t, err, "rg: download failed")
}

func TestHeadlessGateDefaultsToStrict(t *testing.T) {
	// Empty mode + Ask-default bash must fold to Deny (Ask≡Deny).
	policy := permission.DefaultPolicy()
	policy.Mode = "" // unset → headless-strict
	gate, err := HeadlessGate(policy)
	require.NoError(t, err)

	dec, reason := gate.Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "pip install numpy",
	})
	assert.Equal(t, permission.Deny, dec)
	assert.Contains(t, reason, "headless-strict")

	// Allowlisted simple command still allowed.
	dec, _ = gate.Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "pwd",
	})
	assert.Equal(t, permission.Allow, dec)
}

func TestHeadlessGateDangerouslyAllowAll(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.DangerouslyAllowAll = true
	gate, err := HeadlessGate(policy)
	require.NoError(t, err)

	dec, _ := gate.Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "rm -rf /",
	})
	assert.Equal(t, permission.Allow, dec)
}

// A policy the gate cannot compile must stop the headless run rather than
// hand back a permissive gate: `cozyphi run` has no one to ask.
func TestHeadlessGateFailsClosedOnUnusablePolicy(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.BashAllow = []string{"("} // never compiles

	gate, err := HeadlessGate(policy)

	require.Error(t, err)
	assert.Nil(t, gate, "a failed assembly must not return a usable gate")
}

func TestLoadRunBootstrapYolo(t *testing.T) {
	p, _ := testProject(t)
	cfgPath := p.Global().ConfigFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	require.NoError(t, os.WriteFile(cfgPath, []byte(`models:
  - name: m
    api_key: k
permissions:
  mode: headless-strict
`), 0o644))

	bs, err := loadRunBootstrap(t.Context(), p, "", true)
	require.NoError(t, err)
	require.NotNil(t, bs.Gate)

	dec, _ := bs.Gate.Check(t.Context(), permission.Request{
		Action:  permission.ActionBash,
		Command: "pip install numpy",
	})
	assert.Equal(t, permission.Allow, dec)
}

func TestLoadRunBootstrapExplicitWorkspace(t *testing.T) {
	_, pathDir := testProject(t)
	for _, name := range []string{"fd", "rg"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(pathDir, name), []byte("x"), 0o755))
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.Symlink(root, alias))
	p, err := project.Discover(alias)
	require.NoError(t, err)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)
	assert.Equal(t, root, bs.Cwd, "session identity uses the physical supplied workspace")
	assert.Equal(t, project.ProjectSessionDir(p.Global().SessionBase(), root), bs.SessionDir)
	for _, tc := range []struct {
		path string
		want permission.Decision
	}{
		{filepath.Join(root, "new.go"), permission.Allow},
		{filepath.Join(cwd, "new.go"), permission.Deny},
	} {
		decision, _ := bs.Gate.Check(t.Context(), permission.Request{
			Action: permission.ActionWrite,
			Paths:  []string{tc.path},
		})
		assert.Equal(t, tc.want, decision, tc.path)
	}
	after, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, cwd, after, "bootstrap must not change process cwd")

	override := t.TempDir()
	bs, err = loadRunBootstrap(t.Context(), p, override, false)
	require.NoError(t, err)
	assert.Equal(t, override, bs.SessionDir, "explicit storage overrides remain authoritative")
	assert.Equal(t, root, bs.Cwd)
}

func TestHeadlessGateExplicitRoot(t *testing.T) {
	root := t.TempDir()
	policy := permission.DefaultPolicy()
	policy.Mode = ""
	gate, err := HeadlessGate(policy, root)
	require.NoError(t, err)
	decision, _ := gate.Check(t.Context(), permission.Request{
		Action: permission.ActionWrite,
		Paths:  []string{filepath.Join(root, "new.go")},
	})
	assert.Equal(t, permission.Allow, decision)
	decision, _ = gate.Check(t.Context(), permission.Request{
		Action: permission.ActionBash, Command: "pip install numpy",
	})
	assert.Equal(t, permission.Deny, decision, "an explicit root is not a permission bypass")
}

func TestLoadRunBootstrapUnresolvableWorkspaceFailsClosed(t *testing.T) {
	_, _ = testProject(t)
	root := t.TempDir()
	p, err := project.Discover(root)
	require.NoError(t, err)
	require.NoError(t, os.Remove(root))
	for _, yolo := range []bool{false, true} {
		bs, err := loadRunBootstrap(t.Context(), p, "", yolo)
		require.ErrorContains(t, err, "resolve headless workspace")
		assert.Nil(t, bs, "yolo cannot invent a workspace identity")
	}
}

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/project"
)

// testProject discovers a project under a temp HOME so tests never touch the
// real ~/.phi, and returns a project plus a PATH dir for binary stubs.
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

func TestHeadlessGateDefaultsToStrict(t *testing.T) {
	// Empty mode + Ask-default bash must fold to Deny (Ask≡Deny).
	policy := permission.DefaultPolicy()
	policy.Mode = "" // unset → headless-strict
	gate, err := HeadlessGate(policy)
	require.NoError(t, err)

	dec, reason := gate.Check(context.Background(), permission.Request{
		Action:  permission.ActionBash,
		Command: "pip install numpy",
	})
	assert.Equal(t, permission.Deny, dec)
	assert.Contains(t, reason, "headless-strict")

	// Allowlisted simple command still allowed.
	dec, _ = gate.Check(context.Background(), permission.Request{
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

	dec, _ := gate.Check(context.Background(), permission.Request{
		Action:  permission.ActionBash,
		Command: "rm -rf /",
	})
	assert.Equal(t, permission.Allow, dec)
}

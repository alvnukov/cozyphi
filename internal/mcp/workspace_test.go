package mcp_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/mcp"
)

// revive:disable:deep-exit -- The subprocess must exit without testing's PASS/FAIL output corrupting MCP stdout.
// TestWorkspaceCWDHelper is a real stdio MCP server running in the test binary.
func TestWorkspaceCWDHelper(_ *testing.T) {
	if os.Getenv("COZYPHI_TEST_MCP_CWD") != "1" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(3)
		}
		if len(request.ID) == 0 {
			continue
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{{"name": "cwd", "inputSchema": map[string]any{"type": "object"}}},
			}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": cwd}}}
		default:
			os.Exit(4)
		}
		if encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}) != nil {
			os.Exit(5)
		}
	}
	os.Exit(0)
}

// revive:enable:deep-exit

func cwdServerConfig(t *testing.T) map[string]mcp.ServerConfig {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	return map[string]mcp.ServerConfig{"cwd": {
		Command: []string{executable},
		Args:    []string{"-test.run=^TestWorkspaceCWDHelper$"},
		Env:     map[string]string{"COZYPHI_TEST_MCP_CWD": "1"},
	}}
}

func TestLoadPoolInDirBindsWorkspaceProcesses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COZYPHI_MCP", "on")
	servers := cwdServerConfig(t)
	var pools []*mcp.Pool
	var roots []string
	for range 2 {
		root, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		pool, err := mcp.LoadPoolInDir(filepath.Join(root, ".cozyphi", "mcp.json"), root, servers)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, pool.Close()) })
		pools = append(pools, pool)
		roots = append(roots, root)
	}
	for i, pool := range pools {
		got, err := pool.Call(t.Context(), "cwd", "cwd", nil)
		require.NoError(t, err)
		require.Equal(t, roots[i], got)
	}
	// A second call to the first pool must not acquire the second workspace's cwd.
	got, err := pools[0].Call(t.Context(), "cwd", "cwd", nil)
	require.NoError(t, err)
	require.Equal(t, roots[0], got)
}

func TestLoadPoolInDirRejectsInvalidDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	for _, mode := range []string{"on", "off"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("COZYPHI_MCP", mode)
			for _, cwd := range []string{"", filepath.Join(t.TempDir(), "missing"), file} {
				pool, err := mcp.LoadPoolInDir("", cwd, cwdServerConfig(t))
				require.ErrorContains(t, err, "working directory")
				require.Nil(t, pool)
			}
		})
	}
}

func TestLoadPoolInDirPinsCanonicalDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COZYPHI_MCP", "on")
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	other := t.TempDir()
	link := filepath.Join(t.TempDir(), "workspace")
	require.NoError(t, os.Symlink(root, link))
	ambient, err := os.Getwd()
	require.NoError(t, err)
	relative, err := filepath.Rel(ambient, link)
	require.NoError(t, err)
	pool, err := mcp.LoadPoolInDir(filepath.Join(root, "absent.json"), relative, cwdServerConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	// Retarget before the lazy launch, then force a new client generation.
	require.NoError(t, os.Remove(link))
	require.NoError(t, os.Symlink(other, link))
	for range 2 {
		got, err := pool.Call(t.Context(), "cwd", "cwd", nil)
		require.NoError(t, err)
		require.Equal(t, root, got)
		require.NoError(t, pool.SetEnabled("cwd", false))
		require.NoError(t, pool.SetEnabled("cwd", true))
	}
}

func TestPoolCloseRejectsReopening(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "lazy", true: "connected"}[started], func(t *testing.T) {
			pool := mcp.NewPool(cwdServerConfig(t))
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			if started {
				_, err := pool.Call(t.Context(), "cwd", "cwd", nil)
				require.NoError(t, err)
			}
			require.NoError(t, pool.Close())
			require.NoError(t, pool.Close())
			_, err := pool.Call(t.Context(), "cwd", "cwd", nil)
			require.ErrorContains(t, err, "closed")
			_, err = pool.ListTools(t.Context(), "cwd")
			require.ErrorContains(t, err, "closed")
			_, err = pool.Inspect(t.Context(), "cwd", "cwd")
			require.ErrorContains(t, err, "closed")
		})
	}
}

func TestClientCloseRejectsReopening(t *testing.T) {
	// A pool operation can acquire a client just before Pool.Close. That stale
	// reference must not start a new process after the session has been closed.
	client, err := mcp.NewClient("cwd", cwdServerConfig(t)["cwd"])
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Close())
	_, err = client.CallTool(t.Context(), "cwd", nil)
	require.ErrorContains(t, err, "closed")
}

func TestLoadPoolPreservesAmbientDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COZYPHI_MCP", "on")
	ambient, err := os.Getwd()
	require.NoError(t, err)
	pool, err := mcp.LoadPool(filepath.Join(t.TempDir(), "absent.json"), cwdServerConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	got, err := pool.Call(t.Context(), "cwd", "cwd", nil)
	require.NoError(t, err)
	require.Equal(t, ambient, got)
}

func TestLoadPoolInDirMissingAtLaunchFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COZYPHI_MCP", "on")
	root := t.TempDir()
	pool, err := mcp.LoadPoolInDir(filepath.Join(root, "absent.json"), root, cwdServerConfig(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, os.Remove(root))
	got, err := pool.Call(t.Context(), "cwd", "cwd", nil)
	require.Error(t, err)
	require.Empty(t, got, "a missing workspace must not fall back to the ambient directory")
}

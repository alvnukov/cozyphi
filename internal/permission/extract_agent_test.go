package permission_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/permission"
)

func TestExtractAgentAllow(t *testing.T) {
	req, err := permission.Extract("agent_spawn", json.RawMessage(`{"prompt":"x"}`))
	require.NoError(t, err)
	assert.Equal(t, permission.ActionAgent, req.Action)

	g, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	dec, _ := g.Check(t.Context(), req)
	assert.Equal(t, permission.Allow, dec)
}

func TestExtractAgentSpawnWorkdirBecomesPath(t *testing.T) {
	cwd := t.TempDir()

	req, err := permission.ExtractAt("agent_spawn", json.RawMessage(`{"prompt":"x"}`), cwd)
	require.NoError(t, err)
	assert.Empty(t, req.Paths, "spawn without workdir must not carry paths")

	req, err = permission.ExtractAt(
		"agent_spawn",
		json.RawMessage(`{"prompt":"x","workdir":"sub"}`),
		cwd,
	)
	require.NoError(t, err)
	require.Len(t, req.Paths, 1)
	assert.Equal(t, filepath.Join(cwd, "sub"), req.Paths[0], "relative workdir resolves against session cwd")

	req, err = permission.ExtractAt(
		"agent_spawn",
		json.RawMessage(`{"prompt":"x","workdir":"/etc"}`),
		cwd,
	)
	require.NoError(t, err)
	require.Len(t, req.Paths, 1)
	assert.Equal(t, "/etc", req.Paths[0])
}

func TestGateAgentSpawnWorkdirEscapeAsks(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	g, err := permission.NewGate(permission.DefaultPolicy(), ws)
	require.NoError(t, err)

	req, err := permission.ExtractAt(
		"agent_spawn",
		json.RawMessage(`{"prompt":"x","role":"worker","workdir":"`+outside+`"}`),
		ws,
	)
	require.NoError(t, err)
	dec, reason := g.Check(t.Context(), req)
	assert.Equal(t, permission.Ask, dec, "workdir outside the workspace must ask, not allow")
	assert.Contains(t, reason, outside)

	req, err = permission.ExtractAt(
		"agent_spawn",
		json.RawMessage(`{"prompt":"x","role":"worker","workdir":"sub"}`),
		ws,
	)
	require.NoError(t, err)
	dec, reason = g.Check(t.Context(), req)
	assert.Equal(t, permission.Allow, dec)
	assert.Empty(t, reason)

	// Non-interactive modes fold Ask to Deny: a headless parent never gets a
	// silent workspace widening.
	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeHeadlessStrict
	strict, err := permission.NewGate(policy, ws)
	require.NoError(t, err)
	req, err = permission.ExtractAt(
		"agent_spawn",
		json.RawMessage(`{"prompt":"x","workdir":"`+outside+`"}`),
		ws,
	)
	require.NoError(t, err)
	dec, _ = strict.Check(t.Context(), req)
	assert.Equal(t, permission.Deny, dec)
}

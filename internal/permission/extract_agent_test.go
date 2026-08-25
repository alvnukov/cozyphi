package permission_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestExtractAgentAllow(t *testing.T) {
	req, err := permission.Extract("agent_spawn", json.RawMessage(`{"prompt":"x","workdir":"/etc"}`))
	require.NoError(t, err)
	assert.Equal(t, permission.ActionAgent, req.Action)
	assert.Empty(t, req.Paths, "agent tools carry no paths; spawn confinement lives in job.Spawn")

	g, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
	require.NoError(t, err)
	dec, _ := g.Check(t.Context(), req)
	assert.Equal(t, permission.Allow, dec)
}

package permission_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestExtractContextAllow(t *testing.T) {
	for _, args := range []string{`{}`, `{"action":"status"}`, `{"action":"compact"}`} {
		req, err := permission.Extract("context", json.RawMessage(args))
		require.NoError(t, err)
		assert.Equal(t, permission.ActionContext, req.Action)
		assert.Empty(t, req.Paths, "context tool carries no paths; it only reports usage and compacts its own context")
		assert.Empty(t, req.Command)
	}
}

func TestExtractPlanToolCarriesNoExternalCapability(t *testing.T) {
	for _, args := range []string{
		`{"action":"get"}`,
		`{"action":"update","expected_revision":0,"steps":[]}`,
	} {
		req, err := permission.Extract("plan", json.RawMessage(args))
		require.NoError(t, err)
		assert.Equal(t, permission.ActionPlan, req.Action)
		assert.Empty(t, req.Paths)
		assert.Empty(t, req.Command)
	}
}

func TestGatePlanAllowAcrossModes(t *testing.T) {
	req, err := permission.Extract("plan", json.RawMessage(`{"action":"update","expected_revision":0,"steps":[]}`))
	require.NoError(t, err)

	for _, mode := range []permission.Mode{
		permission.ModeInteractive,
		permission.ModeHeadlessStrict,
		permission.ModeAutopilot,
		permission.ModeReadonly,
	} {
		policy := permission.DefaultPolicy()
		policy.Mode = mode
		g, err := permission.NewGate(policy, t.TempDir())
		require.NoError(t, err)
		dec, reason := g.Check(t.Context(), req)
		assert.Equal(t, permission.Allow, dec, "mode %s: %s", mode, reason)
	}
}

func TestGateContextAllowAcrossModes(t *testing.T) {
	req, err := permission.Extract("context", json.RawMessage(`{"action":"compact"}`))
	require.NoError(t, err)

	for _, mode := range []permission.Mode{
		permission.ModeInteractive,
		permission.ModeHeadlessStrict,
		permission.ModeAutopilot,
		permission.ModeReadonly,
	} {
		policy := permission.DefaultPolicy()
		policy.Mode = mode
		g, err := permission.NewGate(policy, t.TempDir())
		require.NoError(t, err)
		dec, reason := g.Check(t.Context(), req)
		assert.Equal(t, permission.Allow, dec, "mode %s: %s", mode, reason)
	}
}

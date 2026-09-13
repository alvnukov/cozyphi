package permission_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestBackgroundBashCannotReuseForegroundAllowlist(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.BashAllow = []string{"^echo\\b"}
	policy.BashDeny = []string{"forbidden"}
	gate, err := permission.NewGate(policy, t.TempDir())
	require.NoError(t, err)
	for _, tc := range []struct {
		args string
		want permission.Decision
	}{
		{`{"command":"echo ok"}`, permission.Allow},
		{`{"command":"echo ok","run_in_background":true}`, permission.Ask},
		{`{"command":"forbidden","run_in_background":true}`, permission.Deny},
	} {
		req, err := permission.ExtractAt("bash", json.RawMessage(tc.args), t.TempDir())
		require.NoError(t, err)
		got, _ := gate.Check(t.Context(), req)
		assert.Equal(t, tc.want, got, tc.args)
	}
}

func TestShellTaskActionRemainsVisibleToWebTaintGate(t *testing.T) {
	gate := taintGate(permission.AllowAll{}, fakeTaint{tainted: true})
	for _, tc := range []struct {
		action string
		want   permission.Decision
	}{
		{"list", permission.Allow}, {"get", permission.Allow}, {"stop", permission.Ask},
	} {
		raw := json.RawMessage(`{"action":"` + tc.action + `","id":"owned"}`)
		req, err := permission.ExtractAt("shell_task", raw, t.TempDir())
		require.NoError(t, err)
		got, _ := gate.Check(t.Context(), req)
		assert.Equal(t, tc.want, got, tc.action)
	}
	req, err := permission.ExtractAt(
		"bash",
		json.RawMessage(`{"command":"echo ok","run_in_background":true}`),
		t.TempDir(),
	)
	require.NoError(t, err)
	got, _ := gate.Check(t.Context(), req)
	assert.Equal(t, permission.Ask, got, "background startup is a mutation even with session-wide allow")
}

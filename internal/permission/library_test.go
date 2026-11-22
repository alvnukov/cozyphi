package permission_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestCodeAndLibraryUseSeparatePermissionDecisions(t *testing.T) {
	policy := permission.DefaultPolicy()
	for _, mode := range []permission.Mode{permission.ModeInteractive, permission.ModeReadonly, permission.ModeHeadlessStrict} {
		policy.Mode = mode
		gate, err := permission.NewGate(policy, t.TempDir())
		require.NoError(t, err)
		for _, n := range []string{"code", "code_wait", "code_test"} {
			code, err := permission.Extract(n, json.RawMessage(`{"source":"return 2"}`))
			require.NoError(t, err)
			decision, _ := gate.Check(t.Context(), code)
			require.Equal(t, permission.Allow, decision)
		}
		for _, op := range append(tooldef.LibraryActions(true), "unknown") {
			args, err := json.Marshal(map[string]string{"action": op, "name": "builds", "version": "1.1.0"})
			require.NoError(t, err)
			req, err := permission.Extract("library", args)
			require.NoError(t, err)
			want := permission.Allow
			if !tooldef.LibraryReadAction(op) {
				want = permission.Ask
				if mode != permission.ModeInteractive {
					want = permission.Deny
				}
			}
			got, _ := gate.Check(t.Context(), req)
			require.Equal(t, want, got, "%s %s", mode, op)
			require.Contains(t, permission.Summarize(req), "builds")
		}
	}
}

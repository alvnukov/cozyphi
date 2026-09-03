package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// The guard is the gate's own verdict, asked a second time. A destination that
// resolves outside the workspace is refused with the gate's reason, so the
// mutation module can name the path in its error.
func TestMutationGuardRefusesDestinationOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	resolvedWS, err := filepath.EvalSymlinks(ws)
	require.NoError(t, err)
	gate, err := permission.NewGate(permission.Policy{WorkspaceOnlyWrites: true}, ws)
	require.NoError(t, err)
	e := NewExecutor(tools.NewRegistry(nil), gate, nil, nil)

	guard := e.mutationGuard("write")
	require.NotNil(t, guard)

	require.NoError(t, guard(t.Context(), filepath.Join(resolvedWS, "note.txt")))

	err = guard(t.Context(), filepath.Join(outside, "note.txt"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside workspace denied")
}

// The edit tool is judged as an edit, not as a write: the reason a refusal
// carries must name the action the model actually called.
func TestMutationGuardNamesTheEditAction(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	gate, err := permission.NewGate(permission.Policy{WorkspaceOnlyWrites: true}, ws)
	require.NoError(t, err)
	e := NewExecutor(tools.NewRegistry(nil), gate, nil, nil)

	err = e.mutationGuard("edit")(t.Context(), filepath.Join(outside, "note.txt"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "edit outside workspace denied")
}

// A sensitive destination is refused by the same seam, so a swap into one
// cannot be laundered through an approved path.
func TestMutationGuardRefusesSensitiveDestination(t *testing.T) {
	ws := t.TempDir()
	secrets := filepath.Join(ws, "secrets")
	require.NoError(t, os.MkdirAll(secrets, 0o755))
	gate, err := permission.NewGate(
		permission.Policy{WorkspaceOnlyWrites: true, SensitivePathDeny: []string{secrets}},
		ws,
	)
	require.NoError(t, err)
	e := NewExecutor(tools.NewRegistry(nil), gate, nil, nil)

	err = e.mutationGuard("write")(t.Context(), filepath.Join(secrets, "id_rsa"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "sensitive path denied")
}

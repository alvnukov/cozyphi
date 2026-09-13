package shelltask_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func TestCloseRemovesOnlyItsPrivateStore(t *testing.T) {
	shared := t.TempDir()
	sentinel := filepath.Join(shared, "other-runtime.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("retain"), 0o600))
	m, err := shelltask.New(shared, func(ctx context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
		<-ctx.Done()
		return proc.Result{Canceled: true}, nil
	})
	require.NoError(t, err)
	result, err := m.Run(t.Context(), request("close", true))
	require.NoError(t, err)
	owned := filepath.Dir(filepath.Dir(result.Snapshot.OutputFile))
	require.NoError(t, m.Close())
	require.NoError(t, m.Close())
	_, err = os.Stat(owned)
	require.True(t, os.IsNotExist(err), "normal shutdown must not accumulate app-private task stores")
	data, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "retain", string(data))
}

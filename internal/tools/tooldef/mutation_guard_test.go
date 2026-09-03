package tooldef

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// A call carrying no guard mutates as before: the seam re-checks a verdict,
// it is never the only place a path is judged.
func TestGuardMutationWithoutGuardAllows(t *testing.T) {
	require.NoError(t, GuardMutation(t.Context(), "/tmp/note.txt"))
}

// The guard reaches the mutation with the path it was asked about, and its
// refusal is what the caller sees.
func TestGuardMutationCarriesTheGuardAndItsRefusal(t *testing.T) {
	var seen string
	ctx := WithMutationGuard(t.Context(), func(_ context.Context, path string) error {
		seen = path
		return errors.New("outside workspace denied")
	})

	err := GuardMutation(ctx, "/ws/pkg/note.txt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "outside workspace denied")
	require.Equal(t, "/ws/pkg/note.txt", seen)
}

// A nil guard is ignored rather than stored, so an unconfigured gate cannot
// panic a write.
func TestWithMutationGuardIgnoresNil(t *testing.T) {
	ctx := WithMutationGuard(t.Context(), nil)
	require.NoError(t, GuardMutation(ctx, "/tmp/note.txt"))
}

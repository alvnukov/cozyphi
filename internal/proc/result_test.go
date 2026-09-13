package proc

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupFailureRemainsVisibleWithExitAndCancellation(t *testing.T) {
	cleanup := errors.New("process group cleanup failed")
	exit := &exec.ExitError{}
	for _, contextErr := range []error{nil, context.Canceled} {
		result, err := classifyRunResult(Result{Output: "captured output"}, exit, cleanup, contextErr)
		require.ErrorIs(t, err, cleanup)
		require.ErrorIs(t, err, exit, "the initial exit cause must also remain inspectable")
		assert.Equal(t, "captured output", result.Output)
	}
	result, err := classifyRunResult(Result{Output: "captured output"}, nil, cleanup, nil)
	require.ErrorIs(t, err, cleanup, "normal shell exit cannot hide failed cleanup")
	assert.Equal(t, "captured output", result.Output)
}

func TestRunResultWithoutCleanupPreservesExitAndCancelContract(t *testing.T) {
	exit := &exec.ExitError{}
	result, err := classifyRunResult(Result{Output: "output"}, exit, nil, nil)
	require.NoError(t, err, "a process exit is a result, not a transport error")
	assert.Equal(t, exit.ExitCode(), result.ExitCode)
	assert.False(t, result.Canceled)
	result, err = classifyRunResult(Result{Output: "output"}, exit, nil, context.Canceled)
	require.NoError(t, err)
	assert.True(t, result.Canceled)
	transport := errors.New("failed to start")
	_, err = classifyRunResult(Result{}, transport, nil, nil)
	require.ErrorIs(t, err, transport)
}

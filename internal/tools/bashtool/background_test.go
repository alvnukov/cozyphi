package bashtool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func TestManagedBashTimeoutAndPromotionContract(t *testing.T) {
	for _, tc := range []struct {
		name, args string
		deadline   time.Duration
		background bool
	}{
		{"background without timeout", `{"command":"echo managed","run_in_background":true}`, 0, true},
		{"background explicit timeout", `{"command":"echo managed","run_in_background":true,"timeout":60}`, time.Minute, true},
		{"foreground default", `{"command":"echo managed"}`, 300 * time.Second, false},
		{"foreground legacy cap", `{"command":"echo managed","timeout":9999}`, time.Hour, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan context.Context, 1)
			manager, err := shelltask.New(
				t.TempDir(),
				func(ctx context.Context, _ proc.Spec, _ proc.Limit) (proc.Result, error) {
					started <- ctx
					<-ctx.Done()
					return proc.Result{Canceled: true}, nil
				},
			)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Close()) })
			tool := InteractiveTool(manager, "parent")
			ctx := tooldef.WithToolCallID(tooldef.WithCwd(t.Context(), t.TempDir()), "call")
			done := make(chan struct {
				result tooldef.Result
				err    error
			}, 1)
			go func() {
				result, err := tool.Run(ctx, json.RawMessage(tc.args))
				done <- struct {
					result tooldef.Result
					err    error
				}{result, err}
			}()
			process := <-started
			deadline, has := process.Deadline()
			assert.Equal(t, tc.deadline > 0, has)
			if has {
				assert.InDelta(t, tc.deadline.Seconds(), time.Until(deadline).Seconds(), 3)
			}
			if !tc.background {
				require.NoError(t, manager.Background(manager.List("parent")[0].ID))
			}
			got := <-done
			require.NoError(t, got.err)
			assert.True(t, strings.HasSuffix(got.result.DeliveryID, ":started"))
			assert.Contains(t, got.result.Content, "output_file:")
			actual, _ := process.Deadline()
			assert.Equal(t, deadline, actual, "promotion preserves the original deadline")
		})
	}
}

func TestManagedBashRejectsInvalidBackgroundTimeoutBeforeLaunch(t *testing.T) {
	m, err := shelltask.New(t.TempDir(), func(context.Context, proc.Spec, proc.Limit) (proc.Result, error) {
		t.Error("invalid input launched a process")
		return proc.Result{}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	for _, seconds := range []int{0, -1, 3601} {
		_, err := InteractiveTool(
			m,
			"parent",
		).Run(t.Context(), json.RawMessage(fmt.Sprintf(`{"command":"echo no","run_in_background":true,"timeout":%d}`, seconds)))
		require.ErrorContains(t, err, "between 1 and 3600")
	}
}

func TestBackgroundCapabilityIsNotAdvertisedOrAcceptedWithoutOwner(t *testing.T) {
	tool := BashTool()
	assert.NotContains(t, tool.Definition.Params.Properties, "run_in_background")
	_, err := tool.Run(t.Context(), json.RawMessage(`{"command":"echo no","run_in_background":true}`))
	require.ErrorContains(t, err, "unavailable")
}

func TestShellTaskGetAndStopAreConversationScoped(t *testing.T) {
	m, err := shelltask.New(t.TempDir(), func(ctx context.Context, spec proc.Spec, _ proc.Limit) (proc.Result, error) {
		spec.Stream("owned output")
		<-ctx.Done()
		return proc.Result{Canceled: true}, nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	result, err := m.Run(
		t.Context(),
		shelltask.Request{
			ParentSessionID: "first",
			ToolUseID:       "call",
			Command:         "echo owned",
			Spec:            proc.Spec{Argv: []string{"unused"}},
			Background:      true,
		},
	)
	require.NoError(t, err)
	for _, action := range []string{"get", "stop"} {
		_, err = TaskTool(
			m,
			"other",
		).Run(t.Context(), json.RawMessage(fmt.Sprintf(`{"action":%q,"id":%q}`, action, result.Snapshot.ID)))
		require.ErrorContains(t, err, "no such task")
	}
	got, err := TaskTool(m, "first").Run(t.Context(), json.RawMessage(`{"action":"list"}`))
	require.NoError(t, err)
	assert.Contains(t, got.Content, result.Snapshot.ID)
}

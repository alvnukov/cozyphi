//go:build !windows

package shelltask_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/shelltask"
)

func TestPromotionKeepsRealShellPIDAndCloseReapsItsGroup(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "release")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	m := newManager(t, nil)
	changed, unsubscribe := m.Subscribe()
	defer unsubscribe()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan shelltask.Result, 1)
	runErrors := make(chan error, 1)
	go func() {
		res, err := m.Run(ctx, shelltask.Request{
			ParentSessionID: "parent",
			ToolUseID:       "real-pid",
			Command:         "read release then report pid",
			Spec: proc.Spec{
				Argv: []string{
					"/bin/sh",
					"-c",
					`exec 3<>"$1"; printf '%s\n' "$$"; read -r ignored <&3; printf '%s\n' "$$"; sleep 30`,
					"sh",
					fifo,
				},
			},
			Timeout: time.Minute,
		})
		result <- res
		runErrors <- err
	}()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	var before shelltask.Snapshot
	for before.Output == "" {
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("shell did not report PID")
		}
		tasks := m.List("parent")
		if len(tasks) > 0 {
			before = tasks[0]
		}
	}
	pid, err := strconv.Atoi(strings.TrimSpace(before.Output))
	require.NoError(t, err)
	require.NoError(t, m.Background(before.ID))
	accepted := <-result
	require.NoError(t, <-runErrors)
	assert.Equal(t, before.ID, accepted.Snapshot.ID)
	assert.Equal(t, before.Deadline, accepted.Snapshot.Deadline)
	cancel()
	writer, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0o600)
	require.NoError(t, err)
	_, err = writer.WriteString("continue\n")
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for {
		tasks := m.List("parent")
		lines := strings.Fields(tasks[0].Output)
		if len(lines) == 2 {
			assert.Equal(t, lines[0], lines[1], "Ctrl+B resumed the identical OS process")
			break
		}
		select {
		case <-changed:
		case <-timeout.C:
			t.Fatal("promoted process did not continue")
		}
	}
	require.NoError(t, m.Close())
	pending, err := m.Pending("parent", 1)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, shelltask.Stopped, pending[0].State)
	assert.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH, "Close waited until the shell was reaped")
}

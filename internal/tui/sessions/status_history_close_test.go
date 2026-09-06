package sessions_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestBeginCloseRetiresHistoryBeforeBackgroundJoin(t *testing.T) {
	bus := controller.NewBus(nil)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	history := controller.NewStatusHistory(
		bus,
		t.TempDir(),
		func(ctx context.Context, _ string, _ time.Time) (session.HistoryStatistics, error) {
			started <- ctx
			<-release // Simulate a scan still unwinding after cancellation.
			return session.HistoryStatistics{}, nil
		},
	)
	view := sessions.NewView(
		nil,
		bus,
		nil,
		nil,
		nil,
		components.DefaultTheme(),
		t.TempDir(),
		"test",
		"",
		1000,
		nil,
		nil,
	)
	view.ConfigureStatusDashboard(history)
	history.Request(7)
	scan := <-started
	// A result queued before retirement must not be accepted while cleanup waits.
	pending := controller.StatusHistoryMsg{Generation: 1}
	require.True(t, history.Accept(pending))
	view.BeginClose()
	defer func() { unblock(); require.NoError(t, view.Close(t.Context())) }()
	require.ErrorIs(
		t,
		scan.Err(),
		context.Canceled,
		"BeginClose must retire history on the UI goroutine before returning",
	)
	require.False(t, history.Accept(pending))
	bus.Publish(pending)
	view.DrainNow()
	history.Request(30) // Retirement must also deny new scans before the join completes.
	select {
	case <-started:
		t.Fatal("retired history admitted another scan")
	default:
	}
}

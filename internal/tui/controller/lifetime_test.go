package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestControllerLifetimeCancelsAllBeforeJoining(t *testing.T) {
	c := &Controller{bus: NewBus(nil), closeBudget: 10 * time.Millisecond}
	first, cancelFirst := context.WithCancel(context.Background())
	second, cancelSecond := context.WithCancel(context.Background())
	defer cancelFirst()
	defer cancelSecond()
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	finishFirst := sync.OnceFunc(func() { close(firstDone) })
	finishSecond := sync.OnceFunc(func() { close(secondDone) })
	t.Cleanup(finishFirst)
	t.Cleanup(finishSecond)
	require.True(t, c.TrackLifetime(cancelFirst, firstDone))
	require.True(t, c.TrackLifetime(cancelSecond, secondDone))

	c.Close()
	require.ErrorIs(t, first.Err(), context.Canceled)
	require.ErrorIs(t, second.Err(), context.Canceled)
	require.False(t, c.TrackLifetime(cancelFirst, firstDone), "disposal closes registration")
	select {
	case <-c.closeDone:
		t.Fatal("caller timeout released incomplete lifetimes")
	default:
	}
	finishFirst()
	select {
	case <-c.closeDone:
		t.Fatal("disposal skipped the second lifetime")
	default:
	}
	finishSecond()
	select {
	case <-c.closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("disposal required another Close")
	}
}

func TestControllerLifetimeRejectsInvalidBarrier(t *testing.T) {
	c := &Controller{bus: NewBus(nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.False(t, c.TrackLifetime(nil, ctx.Done()))
	require.False(t, c.TrackLifetime(cancel, nil))
	require.False(t, (*Controller)(nil).TrackLifetime(cancel, ctx.Done()))
	c.Close()
}

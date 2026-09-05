package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
)

func TestSpawnWithRunnerSelectsAdapterAndSharesSlots(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	runner := func(name string) job.Runner {
		return job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
			started <- name
			select {
			case <-release:
				return name, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
	}
	m := newMgr(t, runner("default"), job.Options{MaxConcurrent: 2})
	req := job.SpawnRequest{Prompt: "run", ParentID: "parent"}

	_, err := m.SpawnWithRunner(t.Context(), req, nil)
	require.ErrorIs(t, err, job.ErrInvalid)
	explicit, err := m.SpawnWithRunner(t.Context(), req, runner("explicit"))
	require.NoError(t, err)
	assert.Equal(t, "explicit", receiveJobSignal(t, started))
	fallback, err := m.Spawn(t.Context(), req)
	require.NoError(t, err)
	assert.Equal(t, "default", receiveJobSignal(t, started))
	_, err = m.SpawnWithRunner(t.Context(), req, runner("overflow"))
	require.ErrorIs(t, err, job.ErrBusy)
	_, err = m.Spawn(t.Context(), req)
	require.ErrorIs(t, err, job.ErrBusy)

	close(release)
	for id, summary := range map[string]string{explicit.ID: "explicit", fallback.ID: "default"} {
		result, err := m.Wait(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, job.StatusCompleted, result.Info.Status)
		assert.Equal(t, summary, result.Summary)
	}
}

func TestCloseParentReapsOnlyOwnedJobsAndKeepsSlotsUntilExit(t *testing.T) {
	started := make(chan context.Context, 2)
	cancelled := make(chan struct{})
	release := make(chan struct{})
	m := newMgr(t, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
		started <- ctx
		<-ctx.Done()
		return "", ctx.Err()
	}), job.Options{MaxConcurrent: 2})
	t.Cleanup(func() { close(release) })

	a, err := m.SpawnWithRunner(t.Context(), job.SpawnRequest{Prompt: "a", ParentID: "a"},
		job.RunnerFunc(func(ctx context.Context, env job.RunEnv) (string, error) {
			started <- ctx
			<-ctx.Done()
			close(cancelled)
			<-release
			return "", env.WriteResult("final teardown write")
		}))
	require.NoError(t, err)
	receiveJobSignal(t, started)
	b, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "b", ParentID: "b"})
	require.NoError(t, err)
	bCtx := receiveJobSignal(t, started)

	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.CloseParent(ctx, "a"), context.DeadlineExceeded)
	receiveJobSignal(t, cancelled)
	assert.NoError(t, bCtx.Err(), "closing a must not cancel b")
	assert.Equal(t, 2, m.LiveCount())
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "closed", ParentID: "a"})
	require.ErrorIs(t, err, job.ErrClosed)
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "full", ParentID: "b"})
	require.ErrorIs(t, err, job.ErrBusy)

	closed := make(chan error, 1)
	go func() { closed <- m.CloseParent(t.Context(), "a") }()
	select {
	case err := <-closed:
		t.Fatalf("parent close returned before runner exit: %v", err)
	default:
	}
	// Release exactly once, including on an earlier assertion failure.
	release <- struct{}{}
	require.NoError(t, receiveJobSignal(t, closed))
	result, err := m.Wait(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCancelled, result.Info.Status)
	assert.Equal(t, "final teardown write", result.Summary)
	events, err := m.Log(t.Context(), a.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Contains(t, events[len(events)-1].Message, "cancelled")
	require.NoError(t, m.CloseParent(t.Context(), "a"))
	assert.NoError(t, bCtx.Err())
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "still usable", ParentID: "b"})
	require.NoError(t, err)
	require.NoError(t, m.CloseParent(t.Context(), "b"))
	result, err = m.Wait(t.Context(), b.ID)
	require.NoError(t, err)
	assert.Equal(t, job.StatusCancelled, result.Info.Status)
}

func TestCloseParentWaitsForInFlightAdmission(t *testing.T) {
	called := make(chan string, 2)
	m := newMgr(t, job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
		called <- env.Job.ParentID
		return "ok", nil
	}), job.Options{MaxConcurrent: 2})
	admitting := &admissionContext{
		Context: t.Context(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	t.Cleanup(func() { close(admitting.release) })
	spawned := make(chan error, 1)
	go func() {
		_, err := m.SpawnWithRunner(admitting, job.SpawnRequest{Prompt: "a", ParentID: "a"},
			job.RunnerFunc(func(_ context.Context, _ job.RunEnv) (string, error) {
				called <- "unexpected a"
				return "", nil
			}))
		spawned <- err
	}()
	receiveJobSignal(t, admitting.entered)

	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.CloseParent(ctx, "a"), context.DeadlineExceeded,
		"close must wait for an admitted spawn's final persistence even before its runner starts")
	_, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "b", ParentID: "b"})
	require.NoError(t, err)
	assert.Equal(t, "b", receiveJobSignal(t, called))
	_, err = m.Spawn(t.Context(), job.SpawnRequest{Prompt: "a again", ParentID: "a"})
	require.ErrorIs(t, err, job.ErrClosed)

	admitting.release <- struct{}{}
	require.ErrorIs(t, receiveJobSignal(t, spawned), job.ErrClosed)
	require.NoError(t, m.CloseParent(t.Context(), "a"))
	infos, err := m.List(t.Context())
	require.NoError(t, err)
	var owned int
	for _, info := range infos {
		if info.ParentID == "a" {
			owned++
			assert.Equal(t, job.StatusCancelled, info.Status)
		}
	}
	assert.Equal(t, 1, owned)
	select {
	case parent := <-called:
		t.Fatalf("unexpected runner invocation: %s", parent)
	default:
	}
}

func TestRunnerCompletionCancelsExecutionContext(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Hour} {
		t.Run(timeout.String(), func(t *testing.T) {
			started := make(chan context.Context, 1)
			release := make(chan struct{})
			m := newMgr(t, job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
				started <- ctx
				select {
				case <-release:
					return "completed", nil
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}), job.Options{})
			info, err := m.Spawn(t.Context(), job.SpawnRequest{Prompt: "finish", Timeout: timeout})
			require.NoError(t, err)
			runCtx := receiveJobSignal(t, started)
			close(release)
			result, err := m.Wait(t.Context(), info.ID)
			require.NoError(t, err)
			assert.Equal(t, job.StatusCompleted, result.Info.Status)
			receiveJobSignal(t, runCtx.Done())
			assert.ErrorIs(t, runCtx.Err(), context.Canceled)
		})
	}
}

// Hold the public context check between store creation and runner registration,
// so the admission/close race is deterministic without a private manager hook.
type admissionContext struct {
	context.Context
	entered chan struct{}
	release chan struct{}
}

func (c *admissionContext) Err() error {
	close(c.entered)
	<-c.release
	return c.Context.Err()
}

func receiveJobSignal[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for job signal")
		var zero T
		return zero
	}
}

//go:build !windows

package voice

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pcmFile writes count samples of the given value as raw s16le, the shape the
// capture command is expected to produce.
func pcmFile(t *testing.T, value int16, count int) string {
	t.Helper()
	raw := make([]byte, count*2)
	for i := range count {
		binary.LittleEndian.PutUint16(raw[i*2:], uint16(value))
	}
	path := filepath.Join(t.TempDir(), "pattern.pcm")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

// fakeCapture builds an argv that emits a fixed PCM pattern, records its pid
// and then blocks forever, standing in for ffmpeg.
func fakeCapture(t *testing.T, samples int) (argv []string, pidPath string) {
	t.Helper()
	pcm := pcmFile(t, 8000, samples)
	pidPath = filepath.Join(t.TempDir(), "pid")
	script := "echo $$ > " + pidPath + "; cat " + pcm + "; exec sleep 30"
	return []string{"sh", "-c", script}, pidPath
}

func waitForSamples(t *testing.T, s Stream, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(s.Samples()) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capture produced %d samples, want %d", len(s.Samples()), want)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: the path is built by the test
		if err == nil && strings.TrimSpace(string(raw)) != "" {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			require.NoError(t, convErr)
			return pid
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capture command never wrote its pid to %s", path)
	return 0
}

func assertProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("capture process %d is still running", pid)
}

func TestCommandCaptureCollectsSamplesAndDrivesTheMeter(t *testing.T) {
	argv, _ := fakeCapture(t, 800)
	stream, err := NewCommandCapture(argv, 60).Start(t.Context(), "default")
	require.NoError(t, err)

	waitForSamples(t, stream, 800)
	assert.Positive(t, stream.Level(), "a loud pattern moves the meter")
	assert.Positive(t, stream.Duration())

	got, err := stream.Stop()
	require.NoError(t, err)
	require.Len(t, got, 800)
	for _, s := range got {
		require.Equal(t, int16(8000), s)
	}
}

func TestCommandCaptureKillsTheProcessOnStop(t *testing.T) {
	argv, pidPath := fakeCapture(t, 400)
	stream, err := NewCommandCapture(argv, 60).Start(t.Context(), "default")
	require.NoError(t, err)

	waitForSamples(t, stream, 400)
	pid := readPID(t, pidPath)
	require.NoError(t, syscall.Kill(pid, 0), "the fake capture is running before Stop")

	_, err = stream.Stop()
	require.NoError(t, err)
	assertProcessGone(t, pid)
}

func TestCommandCaptureKillsTheProcessWhenTheContextIsCanceled(t *testing.T) {
	argv, pidPath := fakeCapture(t, 400)
	ctx, cancel := context.WithCancel(t.Context())
	stream, err := NewCommandCapture(argv, 60).Start(ctx, "default")
	require.NoError(t, err)

	waitForSamples(t, stream, 400)
	pid := readPID(t, pidPath)

	cancel()
	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not finish after the context was canceled")
	}
	assertProcessGone(t, pid)

	got, err := stream.Stop()
	require.NoError(t, err, "audio heard before the cancel is still returned")
	assert.Len(t, got, 400)
}

func TestCommandCaptureStopIsIdempotent(t *testing.T) {
	argv, _ := fakeCapture(t, 400)
	stream, err := NewCommandCapture(argv, 60).Start(t.Context(), "default")
	require.NoError(t, err)

	waitForSamples(t, stream, 400)
	_, err = stream.Stop()
	require.NoError(t, err)
	_, err = stream.Stop()
	require.NoError(t, err)
}

func TestCommandCaptureExplainsADeviceThatProducesNothing(t *testing.T) {
	argv := []string{"sh", "-c", "echo 'Input/output error' >&2; exit 1"}
	capture := &CommandCapture{argv: argv, maxSamples: 16000, goos: "darwin"}
	stream, err := capture.Start(t.Context(), "MacBook Pro Microphone")
	require.NoError(t, err)

	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("capture did not finish after the command exited")
	}

	_, err = stream.Stop()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `capture produced no audio from "MacBook Pro Microphone"`)
	assert.Contains(t, err.Error(), "/voice devices")
	assert.Contains(t, err.Error(), "allow microphone access")
	assert.Contains(t, err.Error(), "Input/output error")
}

func TestCommandCaptureExpandsTheDevicePlaceholder(t *testing.T) {
	out := filepath.Join(t.TempDir(), "argv")
	argv := []string{"sh", "-c", "echo \"$1\" > " + out + "; exec sleep 30", "sh", "{device}"}
	stream, err := NewCommandCapture(argv, 60).Start(t.Context(), "hw:1,0")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = stream.Stop() })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, readErr := os.ReadFile(
			out,
		); readErr == nil &&
			strings.TrimSpace(string(raw)) != "" { //nolint:gosec // G304: the path is built by the test
			assert.Equal(t, "hw:1,0", strings.TrimSpace(string(raw)))
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the capture command never saw the expanded device")
}

func TestCommandCaptureRejectsAnEmptyArgv(t *testing.T) {
	_, err := NewCommandCapture(nil, 60).Start(t.Context(), "default")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no capture command configured")
}

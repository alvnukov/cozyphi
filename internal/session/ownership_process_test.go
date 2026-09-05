package session

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This helper is a separate copy of the test executable, not a goroutine pretending
// to be another process. Its stdin protocol gives the parent deterministic barriers.
func TestSessionOwnerHelper(t *testing.T) {
	path := os.Getenv("COZYPHI_TEST_SESSION_OWNER")
	if path == "" {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	fmt.Println("waiting")
	if !sc.Scan() || sc.Text() != "open" {
		t.Fatal("expected open")
	}
	m, err := OpenSession(path)
	if errors.Is(err, ErrBusy) {
		fmt.Println("busy")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	// Intentionally no deferred Close: crash/kill must rely on the kernel.
	fmt.Println("owned")
	for sc.Scan() {
		switch sc.Text() {
		case "close":
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			fmt.Println("closed")
		case "crash":
			panic("intentional session owner crash")
		case "exit":
			return
		default:
			t.Fatal("unexpected command")
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
}

type ownerProcess struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	lines  chan string
	done   chan struct{}
	err    error // written before done is closed
	stderr bytes.Buffer
}

func startOwnerProcess(t *testing.T, path string) *ownerProcess {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestSessionOwnerHelper$", "-test.timeout=25s")
	cmd.Env = append(os.Environ(), "COZYPHI_TEST_SESSION_OWNER="+path)
	p := &ownerProcess{cmd: cmd, lines: make(chan string, 8), done: make(chan struct{})}
	cmd.Stderr = &p.stderr
	p.input, err = cmd.StdinPipe()
	require.NoError(t, err)
	output, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	go func() {
		sc := bufio.NewScanner(output)
		for sc.Scan() {
			select {
			case p.lines <- sc.Text():
			case <-ctx.Done(): // keep draining until EOF so Wait can reap the child
			}
		}
		p.err = errors.Join(sc.Err(), cmd.Wait())
		close(p.lines)
		close(p.done)
	}()
	t.Cleanup(func() {
		cancel()
		_ = p.input.Close()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Error("session owner subprocess did not exit")
		}
	})
	require.Equal(t, "waiting", p.line(t))
	return p
}

func (p *ownerProcess) send(t *testing.T, command string) {
	t.Helper()
	_, err := fmt.Fprintln(p.input, command)
	require.NoError(t, err)
}

func (p *ownerProcess) line(t *testing.T) string {
	t.Helper()
	select {
	case line, ok := <-p.lines:
		require.True(t, ok, "owner subprocess exited before its response")
		return line
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for session owner subprocess")
		return ""
	}
}

func (p *ownerProcess) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-p.done:
		if p.err != nil {
			return fmt.Errorf("owner subprocess: %w: %s", p.err, p.stderr.String())
		}
		return nil
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for session owner exit")
		return nil
	}
}

func TestSessionOwnershipProcessLifetime(t *testing.T) {
	for _, action := range []string{"close", "kill", "crash", "exit"} {
		t.Run(action, func(t *testing.T) {
			path := writeSessionFixture(t, "")
			p := startOwnerProcess(t, path)
			p.send(t, "open")
			require.Equal(t, "owned", p.line(t))
			_, err := OpenSession(path)
			require.ErrorIs(t, err, ErrBusy)
			list, err := ListSessions(filepath.Dir(path))
			require.NoError(t, err)
			require.Len(t, list, 1)
			require.True(t, list[0].Active)

			switch action {
			case "close":
				p.send(t, "close")
				require.Equal(t, "closed", p.line(t))
				// The process is still alive: Close, not process exit, released it.
			case "kill":
				require.NoError(t, p.cmd.Process.Kill())
				require.Error(t, p.wait(t))
			case "crash":
				p.send(t, "crash")
				require.Error(t, p.wait(t))
			case "exit":
				p.send(t, "exit")
				require.NoError(t, p.wait(t))
			}

			list, err = ListSessions(filepath.Dir(path))
			require.NoError(t, err)
			require.False(t, list[0].Active)
			m, err := OpenSession(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, m.Close()) })
			if action == "close" {
				p.send(t, "exit")
				require.NoError(t, p.wait(t))
			}
		})
	}
}

func TestSessionOwnershipProcessContention(t *testing.T) {
	path := writeSessionFixture(t, "")
	const count = 6
	processes := make([]*ownerProcess, count)
	for i := range processes {
		processes[i] = startOwnerProcess(t, path)
	}
	for _, p := range processes {
		p.send(t, "open")
	}
	var winner *ownerProcess
	for _, p := range processes {
		switch p.line(t) {
		case "owned":
			require.Nil(t, winner, "only one process may acquire ownership")
			winner = p
		case "busy":
			require.NoError(t, p.wait(t))
		default:
			t.Fatal("unexpected ownership response")
		}
	}
	require.NotNil(t, winner)
	require.NoError(t, winner.cmd.Process.Kill())
	require.Error(t, winner.wait(t))
	m, err := OpenSession(path)
	require.NoError(t, err)
	require.NoError(t, m.Close())
}

package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetState clears the latched enable check and the open handle so a test can
// point the log at its own file, and restores that state afterwards.
func resetState(t *testing.T) {
	t.Helper()
	clear := func() {
		mu.Lock()
		defer mu.Unlock()
		if file != nil {
			_ = file.Close()
			file = nil
		}
		checked, enabled = false, false
	}
	clear()
	t.Cleanup(clear)
}

// Path is what a caller shows a human who has to go read the log, so it must
// report the file Logf really opens.
func TestPathPrefersTheEnvironment(t *testing.T) {
	t.Setenv("COZYPHI_DEBUG_FILE", "  /var/log/cozyphi.log  ")
	if got := Path(); got != "/var/log/cozyphi.log" {
		t.Fatalf("Path must trim and use the environment, got %q", got)
	}
	t.Setenv("COZYPHI_DEBUG_FILE", "")
	if got := Path(); got != defaultPath {
		t.Fatalf("Path must fall back to the default, got %q", got)
	}
}

// A multi-line payload (a panic stack, say) has to land in the file whole:
// that file is the only place it exists once the frame is redrawn.
func TestLogfWritesMultiLinePayloadsToTheConfiguredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "1")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	Logf("draw panic: %v\n%s", "width math gone wrong", "goroutine 1 [running]:\napp.drawTree(...)")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the log file must exist: %v", err)
	}
	body := string(raw)
	for _, want := range []string{"width math gone wrong", "goroutine 1 [running]:", "app.drawTree(...)"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the log must keep %q, got %q", want, body)
		}
	}
}

// With the switch off nothing may be written at all: the default path sits in
// the working directory and must not appear because a panic was recovered.
func TestLogfWritesNothingWhenDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	t.Setenv("COZYPHI_DEBUG", "")
	t.Setenv("COZYPHI_DEBUG_FILE", path)
	resetState(t)

	Logf("draw panic: %v", "boom")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the disabled log must not create %s (err %v)", path, err)
	}
}

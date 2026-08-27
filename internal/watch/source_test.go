package watch

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ticker is tested here rather than through Manager because MinInterval
// floors a real spec at five seconds: going through the front door would buy
// a five-second test per assertion and pin the floor, not the semantics.

// replies hands back a different output per call, so a poll can be watched
// changing its mind.
func replies(outputs ...string) (ShellFunc, *atomic.Int32) {
	var calls atomic.Int32
	shell := func(_ context.Context, _ string, onChunk func(string)) (ShellResult, error) {
		n := int(calls.Add(1)) - 1
		if n >= len(outputs) {
			n = len(outputs) - 1
		}
		onChunk(outputs[n])
		return ShellResult{}, nil
	}
	return shell, &calls
}

func drainFor(d time.Duration, run func(ctx context.Context, emit func(string))) []string {
	var (
		mu  sync.Mutex
		out []string
	)
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	run(ctx, func(text string) {
		mu.Lock()
		out = append(out, text)
		mu.Unlock()
	})
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), out...)
}

// TestPollIsSilentUntilTheOutputChanges is the whole point of a polling watch:
// the first run learns what "unchanged" means, and only a difference is news.
func TestPollIsSilentUntilTheOutputChanges(t *testing.T) {
	shell, calls := replies("pending\n", "pending\n", "pending\n", "passed\n")
	src := &ticker{label: "ci", command: "gh pr checks", every: 5 * time.Millisecond, shell: shell}

	got := drainFor(200*time.Millisecond, func(ctx context.Context, emit func(string)) {
		_ = src.Run(ctx, emit)
	})

	require.NotEmpty(t, got, "a change must be reported")
	assert.Equal(t, "passed", got[0])
	assert.Len(t, got, 1, "the same output twice is not two events")
	assert.Positive(t, calls.Load())
}

func TestPollFiltersBeforeComparing(t *testing.T) {
	// Only the ERROR line is watched, so noise around it must not read as a
	// change — otherwise every unrelated log line wakes the session.
	shell, _ := replies("noise 1\n", "noise 2\n", "noise 3\nERROR disk full\n")
	src := &ticker{
		label:   "errors",
		command: "cat log",
		match:   mustCompile(t, "ERROR"),
		every:   5 * time.Millisecond,
		shell:   shell,
	}

	got := drainFor(200*time.Millisecond, func(ctx context.Context, emit func(string)) {
		_ = src.Run(ctx, emit)
	})
	require.Len(t, got, 1)
	assert.Equal(t, "ERROR disk full", got[0])
}

func TestOnceRunsTheCommandAndReportsIt(t *testing.T) {
	shell, calls := replies("all good\n")
	src := &ticker{label: "smoke", command: "./smoke.sh", every: time.Hour, once: true, shell: shell}

	got := drainFor(time.Second, func(ctx context.Context, emit func(string)) {
		require.NoError(t, src.Run(ctx, emit))
	})
	assert.Equal(t, []string{"all good"}, got)
	assert.Equal(t, int32(1), calls.Load(), "once means once, not once an hour")
}

func TestBareTimerWaitsOutItsIntervalThenSaysItsLabel(t *testing.T) {
	src := &ticker{label: "check the deploy", every: 20 * time.Millisecond}

	immediate := drainFor(5*time.Millisecond, func(ctx context.Context, emit func(string)) {
		_ = src.Run(ctx, emit)
	})
	assert.Empty(t, immediate, "a reminder that fires at once is not a reminder")

	got := drainFor(120*time.Millisecond, func(ctx context.Context, emit func(string)) {
		_ = src.Run(ctx, emit)
	})
	require.NotEmpty(t, got)
	assert.Equal(t, "check the deploy", got[0])
}

func TestOneShotTimerFiresOnce(t *testing.T) {
	src := &ticker{label: "stand up", every: 10 * time.Millisecond, once: true}
	got := drainFor(200*time.Millisecond, func(ctx context.Context, emit func(string)) {
		require.NoError(t, src.Run(ctx, emit))
	})
	assert.Equal(t, []string{"stand up"}, got)
}

// TestLineSplitterRebuildsLinesAcrossChunks pins the part a real shell makes
// hard to see: process output arrives in arbitrary chunks, and only the
// splitter knows where a line ends.
func TestLineSplitterRebuildsLinesAcrossChunks(t *testing.T) {
	var got []string
	s := &lineSplitter{fn: func(line string) { got = append(got, line) }}
	s.write("hel")
	s.write("lo\r\nwor")
	s.write("ld\n\n  \n")
	s.write("no trailing newline")
	s.close()

	assert.Equal(t, []string{"hello", "world", "no trailing newline"}, got, "blank lines are not events")
}

func TestLineSplitterDropsAnEndlessLine(t *testing.T) {
	var got []string
	s := &lineSplitter{fn: func(line string) { got = append(got, line) }}
	s.write(strings.Repeat("x", maxLineBytes*3))
	s.write("\n")

	require.Len(t, got, 1)
	assert.Len(t, got[0], maxLineBytes, "a line is cut at the cap, not buffered forever")
}

func TestExitReportLeadsWithTheVerdict(t *testing.T) {
	assert.Equal(t, "go test ./... succeeded", exitReport("go test ./...", 0, nil))

	report := exitReport("make lint", 1, []string{"a.go:1: bad", "b.go:2: worse"})
	assert.True(t, strings.HasPrefix(report, "make lint failed with exit 1"))
	assert.Contains(t, report, "b.go:2: worse")
}

func mustCompile(t *testing.T, expr string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(expr)
	require.NoError(t, err)
	return re
}

package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
)

// A recovered frame is only diagnosable from its stack, so the stack must
// reach the debug log even though the screen shows one line.
func TestDrawTreeLogsThePanicWithItsStack(t *testing.T) {
	var logged []string
	prev := logPanic
	logPanic = func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { logPanic = prev })

	a := &App{root: &panicRoot{}}
	a.drawTree(components.DrawContext{Max: components.Size{Width: 40, Height: 10}})

	if len(logged) != 1 {
		t.Fatalf("a recovered frame must log once, got %d entries", len(logged))
	}
	entry := logged[0]
	if !strings.Contains(entry, "width math gone wrong") {
		t.Fatalf("the log entry must name the panic, got %q", entry)
	}
	if !strings.Contains(entry, "drawTree") || !strings.Contains(entry, ".go:") {
		t.Fatalf("the log entry must carry the stack, got %q", entry)
	}
}

// The one line left on screen has to say where the stack went, and must not
// name a log file when nothing was written to it.
func TestNoticeTextSaysWhereTheStackIs(t *testing.T) {
	on := noticeText("boom", true, "/tmp/cozyphi-debug.log")
	if !strings.Contains(on, "boom") || !strings.Contains(on, "/tmp/cozyphi-debug.log") {
		t.Fatalf("with the log on the notice must name the panic and the file, got %q", on)
	}

	off := noticeText("boom", false, "/tmp/cozyphi-debug.log")
	if strings.Contains(off, "/tmp/cozyphi-debug.log") {
		t.Fatalf("with the log off the notice must not point at an empty file, got %q", off)
	}
	if !strings.Contains(off, "COZYPHI_DEBUG") {
		t.Fatalf("with the log off the notice must say how to capture a stack, got %q", off)
	}
}

// The notice is longer than a narrow pane, so it wraps instead of being cut at
// the right edge and losing the part that says where to look.
func TestErrorSurfaceWrapsTheNoticeAcrossLines(t *testing.T) {
	msg := noticeText("width math gone wrong", true, "cozyphi-debug.log")
	surf := errorSurface(components.DrawContext{Max: components.Size{Width: 24, Height: 8}}, msg)

	text := components.ExtractSurfaceText(surf, 0, 0, surf.Size.Width, surf.Size.Height)
	flat := strings.Join(strings.Fields(text), " ")
	if want := strings.Join(strings.Fields(msg), " "); !strings.Contains(flat, want) {
		t.Fatalf("the notice must survive a narrow screen whole, got %q", flat)
	}
}

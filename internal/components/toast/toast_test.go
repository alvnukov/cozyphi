package toast

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func expire(toast *Toast) {
	toast.Until = time.Now().Add(-time.Millisecond)
}

func TestToastQueueShowsMessagesInTurn(t *testing.T) {
	var toast Toast
	toast.Show("first", ToastSuccess, time.Second)
	toast.Show("second", ToastError, 5*time.Second)

	if !toast.Visible() || toast.Message != "first" {
		t.Fatalf("visible=%v message=%q, want the first toast up", toast.Visible(), toast.Message)
	}

	expire(&toast)
	if !toast.Visible() || toast.Message != "second" || toast.Kind != ToastError {
		t.Fatalf("message=%q kind=%v, want the queued error next", toast.Message, toast.Kind)
	}

	expire(&toast)
	if toast.Visible() {
		t.Fatal("an empty queue leaves the slot empty")
	}
}

func TestToastErrorIsNotCutShortByInfo(t *testing.T) {
	var toast Toast
	toast.Show("disk full", ToastError, 5*time.Second)
	toast.Show("copied", ToastSuccess, time.Second)

	if toast.Message != "disk full" {
		t.Fatalf("message=%q, the info toast must wait its turn", toast.Message)
	}
	if until := time.Until(toast.Until); until < 4*time.Second {
		t.Fatalf("the error's remaining time shrank to %v", until)
	}
}

func TestToastClearDismissesTheCurrentOnly(t *testing.T) {
	var toast Toast
	toast.Show("first", ToastSuccess, time.Second)
	toast.Show("second", ToastWarning, time.Second)

	toast.Clear()
	if !toast.Visible() || toast.Message != "second" {
		t.Fatalf("message=%q, Clear must hand the slot to the queue", toast.Message)
	}
}

func TestToastHistoryIsNewestFirstAndCapped(t *testing.T) {
	var toast Toast
	for i := range 25 {
		toast.Show(string(rune('a'+i)), ToastSuccess, time.Second)
	}
	history := toast.History()
	if len(history) != 20 {
		t.Fatalf("history length=%d, want the cap", len(history))
	}
	if history[0].Message != "y" || history[19].Message != "f" {
		t.Fatalf("history order %q..%q, want newest first", history[0].Message, history[19].Message)
	}
}

func TestToastDrawSuccess(t *testing.T) {
	toast := Toast{Theme: components.DefaultTheme()}
	toast.Show("Selection copied to clipboard", ToastSuccess, time.Second)
	s := toast.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}, Method: xui.WidthUnicode})
	if len(s.Children) != 1 {
		t.Fatalf("children=%d", len(s.Children))
	}
	panel := s.Children[0].Surface
	var row strings.Builder
	for x := 0; x < panel.Size.Width; x++ {
		ch := panel.Buffer[panel.Size.Width+x].Char // y=1 content row
		if ch == "" {
			ch = " "
		}
		row.WriteString(ch)
	}
	got := row.String()
	if !strings.Contains(got, "Selection copied") {
		t.Fatalf("toast row=%q", got)
	}
	if !strings.Contains(got, "✓") {
		t.Fatalf("missing checkmark: %q", got)
	}
}

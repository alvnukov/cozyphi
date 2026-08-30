package notify_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/notify"
)

type delivery struct {
	title string
	body  string
}

// fakeSender records deliveries and can block or fail.
type fakeSender struct {
	mu         sync.Mutex
	deliveries []delivery
	block      chan struct{} // non-nil: each send waits until closed
	fail       bool
	called     chan struct{} // signals when a send starts
	returned   chan struct{} // signals when a send returns
}

func (f *fakeSender) send(_ context.Context, title, body string) error {
	if f.called != nil {
		f.called <- struct{}{}
	}
	defer func() {
		if f.returned != nil {
			f.returned <- struct{}{}
		}
	}()
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("sender failed")
	}
	f.deliveries = append(f.deliveries, delivery{title: title, body: body})
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deliveries)
}

func delivered(t *testing.T, f *fakeSender, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for f.count() != want {
		select {
		case <-deadline:
			t.Fatalf("waiting for %d deliveries, got %d", want, f.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestParseMode(t *testing.T) {
	cases := []struct {
		in      string
		want    notify.Mode
		wantErr bool
	}{
		{in: "off", want: notify.ModeOff},
		{in: "always", want: notify.ModeAlways},
		{in: "unfocused", want: notify.ModeUnfocused},
		{in: "", wantErr: true},
		{in: "sometimes", wantErr: true},
		{in: "OFF", wantErr: true},
	}
	for _, tc := range cases {
		got, err := notify.ParseMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q) = %v, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMode(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseMode(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestModeGating(t *testing.T) {
	t.Run("off sends nothing", func(t *testing.T) {
		f := &fakeSender{}
		n := notify.New(notify.ModeOff, notify.WithSender(f.send))
		n.TurnEnded()
		n.NeedsAttention("permission: Bash")
		delivered(t, f, 0)
	})
	t.Run("always sends when focused", func(t *testing.T) {
		f := &fakeSender{}
		n := notify.New(notify.ModeAlways, notify.WithSender(f.send))
		n.SetFocused(true)
		n.TurnEnded()
		delivered(t, f, 1)
	})
	t.Run("unfocused stays silent while focused by default", func(t *testing.T) {
		f := &fakeSender{}
		n := notify.New(notify.ModeUnfocused, notify.WithSender(f.send))
		n.TurnEnded()
		delivered(t, f, 0)
	})
	t.Run("unfocused sends after focus is lost", func(t *testing.T) {
		f := &fakeSender{}
		n := notify.New(notify.ModeUnfocused, notify.WithSender(f.send))
		n.SetFocused(false)
		n.NeedsAttention("permission: Bash")
		delivered(t, f, 1)
	})
}

func TestTitlesAndBodies(t *testing.T) {
	f := &fakeSender{}
	n := notify.New(notify.ModeAlways, notify.WithSender(f.send))
	n.TurnEnded()
	delivered(t, f, 1)
	n.NeedsAttention("")
	delivered(t, f, 2)
	n.NeedsAttention("permission: Bash(ls)")
	delivered(t, f, 3)

	f.mu.Lock()
	defer f.mu.Unlock()
	wantBodies := []string{
		"Turn finished — waiting for input",
		"The model is waiting for your input",
		"permission: Bash(ls)",
	}
	for i, d := range f.deliveries {
		if d.title != "cozyphi" {
			t.Errorf("delivery %d title = %q, want cozyphi", i, d.title)
		}
		if d.body != wantBodies[i] {
			t.Errorf("delivery %d body = %q, want %q", i, d.body, wantBodies[i])
		}
	}
}

func TestBusyDrop(t *testing.T) {
	f := &fakeSender{block: make(chan struct{}), called: make(chan struct{}, 4)}
	n := notify.New(notify.ModeAlways, notify.WithSender(f.send))

	n.NeedsAttention("first")
	<-f.called                 // first send is now in flight and blocked
	n.NeedsAttention("second") // dropped: one send in flight

	close(f.block)
	delivered(t, f, 1)
	// Give the dropped send a chance to (wrongly) arrive.
	time.Sleep(50 * time.Millisecond)
	if got := f.count(); got != 1 {
		t.Fatalf("busy drop failed: got %d deliveries, want 1", got)
	}

	// After the in-flight send finishes, new notifications flow again.
	n.NeedsAttention("third")
	delivered(t, f, 2)
}

func TestFailureSuppression(t *testing.T) {
	f := &fakeSender{fail: true, called: make(chan struct{}, 4), returned: make(chan struct{}, 4)}
	n := notify.New(notify.ModeAlways, notify.WithSender(f.send))

	n.TurnEnded()
	<-f.called
	<-f.returned // the failing send has returned; suppression is now armed

	// The first failure disables the sender; later calls must not even start.
	n.TurnEnded()
	select {
	case <-f.called:
		t.Fatal("notification attempted after a sender failure")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNilReceiverIsSafe(_ *testing.T) {
	var n *notify.Notifier
	n.SetFocused(true)
	n.TurnEnded()
	n.NeedsAttention("permission: Bash") // must not panic
}

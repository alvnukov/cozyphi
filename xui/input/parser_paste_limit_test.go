package input

import (
	"strings"
	"testing"
)

func TestRejectedPasteDrainsAndRecovers(t *testing.T) {
	for split := 1; split < len(bracketedPasteEnd); split++ {
		p := NewParser()
		if events := p.Feed([]byte("\x1b[200~" + strings.Repeat("x", MaxPasteBytes+1))); len(events) != 0 {
			t.Fatal(events)
		}
		for range 100 {
			if events := p.Feed([]byte("\r\n\x03\x1b[200~\x1b[201X")); len(events) != 0 {
				t.Fatal(events)
			}
			if len(p.pasteBuf) != 0 || len(p.buf) >= len(bracketedPasteEnd) {
				t.Fatal("discard retained payload")
			}
			if p.Pending() || len(p.FlushIdle()) != 0 {
				t.Fatal("discard exposed idle key")
			}
		}
		if events := p.Feed(bracketedPasteEnd[:split]); len(events) != 0 {
			t.Fatal(events)
		}
		events := p.Feed(append(append([]byte{}, bracketedPasteEnd[split:]...), []byte("\x1b[200~ok\x1b[201~\r")...))
		if len(events) != 3 {
			t.Fatalf("got %d events", len(events))
		}
		if _, ok := events[0].(PasteRejectedEvent); !ok {
			t.Fatalf("got %T", events[0])
		}
		if paste, ok := events[1].(PasteEvent); !ok || paste.Text != "ok" {
			t.Fatal(events[1])
		}
		if key, ok := events[2].(KeyEvent); !ok || key.Code != KeyEnter {
			t.Fatal(events[2])
		}
	}
}

func TestPasteLimitCountsRawBytesAndResetClearsDiscard(t *testing.T) {
	p := NewParser()
	events := p.Feed([]byte("\x1b[200~" + strings.Repeat("\r\n", MaxPasteBytes/2+1) + "\x1b[201~"))
	if len(events) != 1 {
		t.Fatal(len(events))
	}
	if _, ok := events[0].(PasteRejectedEvent); !ok {
		t.Fatalf("got %T", events[0])
	}
	p.Feed([]byte("\x1b[200~" + strings.Repeat("x", MaxPasteBytes+1)))
	p.Reset()
	events = p.Feed([]byte("\x1b[200~🙂\x1b[201~"))
	if len(events) != 1 {
		t.Fatal(len(events))
	}
	if paste, ok := events[0].(PasteEvent); !ok || paste.Text != "🙂" {
		t.Fatal(events[0])
	}
}

package input

import (
	"fmt"
	"strings"
	"testing"
)

func TestParserLargeBracketedPaste(t *testing.T) {
	for _, size := range []int{(1 << 20) - 1, 1 << 20, (1 << 20) + 1, 2 << 20} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			p := NewParser()
			p.Feed([]byte("\x1b[200~"))
			const tail = "\nхвост🙂"
			text := strings.Repeat("a", size-len(tail)) + tail
			for offset := 0; offset < len(text); offset += 4096 {
				events := p.Feed([]byte(text[offset:min(offset+4096, len(text))]))
				if len(events) != 0 {
					t.Fatalf("paste emitted %d events before end marker at byte %d", len(events), offset)
				}
			}
			if events := p.Feed([]byte("\x1b[20")); len(events) != 0 {
				t.Fatalf("partial end marker emitted %d events", len(events))
			}
			events := p.Feed([]byte("1~"))
			if len(events) != 1 {
				t.Fatalf("got %d events, want one paste", len(events))
			}
			if size > MaxPasteBytes {
				if _, ok := events[0].(PasteRejectedEvent); !ok {
					t.Fatalf("expected rejection, got %T", events[0])
				}
			} else {
				paste, ok := events[0].(PasteEvent)
				if !ok || paste.Text != text {
					t.Fatalf("paste corrupted: event %T, got %d bytes, want %d", events[0], len(paste.Text), len(text))
				}
			}
			events = p.Feed([]byte("\r"))
			if len(events) != 1 {
				t.Fatalf("got %d events after Enter", len(events))
			}
			key, ok := events[0].(KeyEvent)
			if !ok || key.Code != KeyEnter || !key.Press {
				t.Fatalf("expected Enter after paste, got %T", events[0])
			}
		})
	}
}

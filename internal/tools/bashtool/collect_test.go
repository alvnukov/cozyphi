package bashtool

import (
	"testing"
)

func TestBashOutputTailKeepsNewestLinesAndBytes(t *testing.T) {
	tail := NewBashOutputTail(3, 64)
	if _, err := tail.WriteString("one\ntwo\nthree\nfour\n"); err != nil {
		t.Fatal(err)
	}
	got, truncated := tail.Snapshot()
	if want := "two\nthree\nfour\n"; got != want || !truncated {
		t.Fatalf("got %q truncated=%v, want %q and truncation", got, truncated, want)
	}

	tail = NewBashOutputTail(100, 10)
	if _, err := tail.WriteString("0123456789ABC"); err != nil {
		t.Fatal(err)
	}
	got, truncated = tail.Snapshot()
	if got != "3456789ABC" || !truncated {
		t.Fatalf("got %q truncated=%v, want newest 10 bytes", got, truncated)
	}
}

package components

import (
	"testing"
	"time"
)

func TestDrawContextWakeMinMerge(t *testing.T) {
	var wake time.Time
	ctx := DrawContext{Wake: &wake}
	early := time.Now().Add(500 * time.Millisecond)
	late := early.Add(time.Second)
	ctx.WakeAt(late)
	ctx.WakeAt(early)
	if !wake.Equal(early) {
		t.Fatalf("wake = %v, want earliest %v", wake, early)
	}
	// Zero WakeAt must not clobber the merged value.
	ctx.WakeAt(time.Time{})
	if !wake.Equal(early) {
		t.Fatalf("zero WakeAt clobbered wake: %v", wake)
	}
}

// A zero DrawContext (tests, standalone draws) makes wake publishing a no-op.
func TestDrawContextWakeNilSafe(_ *testing.T) {
	DrawContext{}.WakeAt(time.Now())
	DrawContext{}.WakeIn(time.Second)
}

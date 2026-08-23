package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func paintFrame(r *Renderer, w *mockWriter, cursorX, cursorY int, visible bool, shape int) (int, error) {
	dirty := []cell.DirtyCell{
		{X: 0, Y: 0, Cell: cell.Cell{Char: "A", Width: 1}},
	}
	return r.RenderDiff(w, dirty, cursorX, cursorY, visible, shape)
}

// An unchanged frame (no dirty cells, same cursor state) must write zero
// bytes: no SGR reset, no hide/show cycle. This is the idle-jitter pin.
func TestRenderDiffIdleFrameWritesNothing(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	buf.b = buf.b[:0]
	n, err := r.RenderDiff(&buf, nil, 0, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(buf.b) != 0 {
		t.Fatalf("idle frame wrote %d bytes: %q", n, buf.b)
	}
	// Still quiet on a third identical call.
	n, err = r.RenderDiff(&buf, nil, 0, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(buf.b) != 0 {
		t.Fatalf("third idle frame wrote %d bytes: %q", n, buf.b)
	}
}

// Moving the cursor without painting must emit only the move sequence —
// no SGR reset, no hide/show flicker.
func TestRenderDiffCursorOnlyMove(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	buf.b = buf.b[:0]
	n, err := r.RenderDiff(&buf, nil, 3, 2, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	if n == 0 || out != csi+"3;4H" {
		t.Fatalf("cursor-only move emitted %q", out)
	}
}

// Hiding a visible cursor with nothing to paint emits exactly one hide.
func TestRenderDiffHideTransition(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	buf.b = buf.b[:0]
	n, err := r.RenderDiff(&buf, nil, 0, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 || string(buf.b) != seqHideCursor {
		t.Fatalf("hide transition emitted %q", buf.b)
	}
}

// Painting frames keep the hide → cells → move/show bracket, even when the
// cursor state did not change (it was hidden for the paint).
func TestRenderDiffPaintingFrameBracketsCursor(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	buf.b = buf.b[:0]
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	hideIdx := strings.Index(out, seqHideCursor)
	showIdx := strings.LastIndex(out, seqShowCursor)
	if hideIdx < 0 || showIdx < 0 || showIdx < hideIdx {
		t.Fatalf("painting frame lacks hide/show bracket: %q", out)
	}
	if !strings.Contains(out, "A") {
		t.Fatalf("painting frame missing glyph: %q", out)
	}
}

// A failed write invalidates the cursor cache: the next identical frame
// must re-establish full cursor state instead of skipping.
func TestRenderDiffErrorInvalidatesCache(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	failing := &failWriter{err: errors.New("tty gone")}
	if _, err := r.RenderDiff(failing, nil, 3, 2, true, 0); err == nil {
		t.Fatal("expected write error")
	}
	buf.b = buf.b[:0]
	n, err := r.RenderDiff(&buf, nil, 3, 2, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	if n == 0 || !strings.Contains(out, seqShowCursor) {
		t.Fatalf("post-error frame did not re-establish cursor: %q", out)
	}
}

// ResetState (resize / alt-screen transitions) must forget the cursor
// cache so the next frame re-emits cursor state.
func TestResetStateForgetsCursor(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	if _, err := paintFrame(r, &buf, 0, 0, true, 0); err != nil {
		t.Fatal(err)
	}
	r.ResetState()
	buf.b = buf.b[:0]
	n, err := r.RenderDiff(&buf, nil, 0, 0, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	if n == 0 || !strings.Contains(out, seqShowCursor) {
		t.Fatalf("post-reset frame did not re-establish cursor: %q", out)
	}
}

type failWriter struct{ err error }

func (f *failWriter) Write(p []byte) (int, error) { return 0, f.err }

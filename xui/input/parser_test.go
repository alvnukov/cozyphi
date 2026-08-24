package input

import "testing"

func TestParserSimpleKey(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("a"))
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	k, ok := evs[0].(KeyEvent)
	if !ok || k.Rune != 'a' {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserCtrlC(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte{0x03})
	k := evs[0].(KeyEvent)
	if !k.CtrlC() {
		t.Fatalf("expected ctrl+c, got %#v", k)
	}
}

func TestParserArrowCSI(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[A"))
	if len(evs) != 1 {
		t.Fatalf("got %d", len(evs))
	}
	k := evs[0].(KeyEvent)
	if k.Code != KeyUp {
		t.Fatalf("got %#v", k)
	}
}

func TestParserIncomplete(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte{0x1b, '['})
	if len(evs) != 0 {
		t.Fatalf("expected buffer, got %#v", evs)
	}
	evs = p.Feed([]byte{'B'})
	if len(evs) != 1 {
		t.Fatalf("got %d", len(evs))
	}
	if evs[0].(KeyEvent).Code != KeyDown {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserSGRMouse(t *testing.T) {
	p := NewParser()
	// ESC [ < 0 ; 5 ; 10 M  => left press at (4,9)
	evs := p.Feed([]byte("\x1b[<0;5;10M"))
	if len(evs) != 1 {
		t.Fatalf("got %d %#v", len(evs), evs)
	}
	m := evs[0].(MouseEvent)
	if m.Button != MouseLeft || m.X != 4 || m.Y != 9 || m.Action != MousePress {
		t.Fatalf("got %#v", m)
	}
}

func TestParserX10MouseWheelNotKeyRunes(t *testing.T) {
	p := NewParser()
	// Wheel up at (26,22): Cb=64+32='`', Cx=27+32=';', Cy=23+32='7'
	// Without X10 handling this leaked as KeyRunes '`', ';', '7' into the composer.
	evs := p.Feed([]byte{0x1b, '[', 'M', '`', ';', '7'})
	if len(evs) != 1 {
		t.Fatalf("got %d %#v", len(evs), evs)
	}
	m, ok := evs[0].(MouseEvent)
	if !ok || m.Button != MouseWheelUp || m.X != 26 || m.Y != 22 || m.Wheel != 1 {
		t.Fatalf("got %#v", evs[0])
	}

	p = NewParser()
	// Wheel down: Cb=65+32='a'
	evs = p.Feed([]byte{0x1b, '[', 'M', 'a', ';', '7'})
	if len(evs) != 1 {
		t.Fatalf("got %d %#v", len(evs), evs)
	}
	m, ok = evs[0].(MouseEvent)
	if !ok || m.Button != MouseWheelDown || m.X != 26 || m.Y != 22 {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserX10MouseChunked(t *testing.T) {
	p := NewParser()
	if evs := p.Feed([]byte{0x1b, '[', 'M'}); len(evs) != 0 {
		t.Fatalf("partial should wait, got %#v", evs)
	}
	evs := p.Feed([]byte{'`', ';', '7'})
	if len(evs) != 1 {
		t.Fatalf("got %d %#v", len(evs), evs)
	}
	if _, ok := evs[0].(MouseEvent); !ok {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserDA1(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[?62;c"))
	if len(evs) != 1 {
		t.Fatalf("got %d", len(evs))
	}
	if _, ok := evs[0].(CapEvent); !ok {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserKittyKBQueryReply(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[?0u"))
	if len(evs) != 1 {
		t.Fatalf("got %d %#v", len(evs), evs)
	}
	c, ok := evs[0].(CapEvent)
	if !ok || c.Kind != CapKittyKB {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestParserCSIu(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[97;5u")) // 'a' with ctrl (mods=5 → ctrl)
	k := evs[0].(KeyEvent)
	if k.Rune != 'a' || !k.Mods.Has(ModCtrl) {
		t.Fatalf("got %#v", k)
	}
}

func TestParserShiftEnterCSIu(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[13;2u")) // tmux Shift+Enter
	if len(evs) != 1 {
		t.Fatalf("got %d", len(evs))
	}
	k := evs[0].(KeyEvent)
	if k.Code != KeyEnter || !k.Mods.Has(ModShift) || !k.Press {
		t.Fatalf("got %#v", k)
	}
}

func TestParserShiftEnterModifyOtherKeys(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[27;2;13~"))
	if len(evs) != 1 {
		t.Fatalf("got %d", len(evs))
	}
	k := evs[0].(KeyEvent)
	if k.Code != KeyEnter || !k.Mods.Has(ModShift) {
		t.Fatalf("got %#v", k)
	}
}

func TestParserAltEnterCSIu(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[13;3u")) // Alt+Enter (mods=3 → alt)
	k := evs[0].(KeyEvent)
	if k.Code != KeyEnter || !k.Mods.Has(ModAlt) {
		t.Fatalf("got %#v", k)
	}
}

func TestParserKittyRelease(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[13;2:3u"))
	k := evs[0].(KeyEvent)
	if k.Code != KeyEnter || k.Press {
		t.Fatalf("expected release, got %#v", k)
	}
}

func TestParserKittyArrowPressRelease(t *testing.T) {
	p := NewParser()
	// Kitty reportEventTypes: CSI 1 ; mods : event_type B
	press := p.Feed([]byte("\x1b[1;1:1B"))
	if len(press) != 1 {
		t.Fatalf("press events=%d %#v", len(press), press)
	}
	k := press[0].(KeyEvent)
	if k.Code != KeyDown || !k.Press || k.Mods != 0 {
		t.Fatalf("press got %#v", k)
	}
	rel := p.Feed([]byte("\x1b[1;1:3B"))
	if len(rel) != 1 {
		t.Fatalf("release events=%d %#v", len(rel), rel)
	}
	k = rel[0].(KeyEvent)
	if k.Code != KeyDown || k.Press {
		t.Fatalf("release got %#v", k)
	}
}

func TestParserKittyArrowShift(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[1;2:1A"))
	k := evs[0].(KeyEvent)
	if k.Code != KeyUp || !k.Press || !k.Mods.Has(ModShift) {
		t.Fatalf("got %#v", k)
	}
}

func TestParserKittyPageUpRelease(t *testing.T) {
	p := NewParser()
	evs := p.Feed([]byte("\x1b[5;1:3~"))
	k := evs[0].(KeyEvent)
	if k.Code != KeyPageUp || k.Press {
		t.Fatalf("expected PageUp release, got %#v", k)
	}
}

func TestParserBracketedPasteMultiline(t *testing.T) {
	p := NewParser()
	// Mid-paste newlines must not become KeyEnter / submit.
	raw := "\x1b[200~line1\r\nline2\nline3\x1b[201~"
	evs := p.Feed([]byte(raw))
	if len(evs) != 1 {
		t.Fatalf("got %d events: %#v", len(evs), evs)
	}
	pe, ok := evs[0].(PasteEvent)
	if !ok {
		t.Fatalf("got %#v", evs[0])
	}
	if pe.Text != "line1\nline2\nline3" {
		t.Fatalf("text=%q", pe.Text)
	}
}

func TestParserBracketedPasteChunked(t *testing.T) {
	p := NewParser()
	var all []Event
	all = append(all, p.Feed([]byte("\x1b[200~hello"))...)
	all = append(all, p.Feed([]byte("\nworld\x1b[20"))...) // partial end
	all = append(all, p.Feed([]byte("1~"))...)
	if len(all) != 1 {
		t.Fatalf("got %d events: %#v", len(all), all)
	}
	pe := all[0].(PasteEvent)
	if pe.Text != "hello\nworld" {
		t.Fatalf("text=%q", pe.Text)
	}
}

func TestParserLoneEscFlushedOnIdle(t *testing.T) {
	p := NewParser()
	// A bare Esc is a complete key press, not a sequence prefix — but only
	// time can prove no follow-up byte is coming, so Feed holds it.
	if evs := p.Feed([]byte{0x1b}); len(evs) != 0 {
		t.Fatalf("expected hold, got %#v", evs)
	}
	if !p.Pending() {
		t.Fatal("expected pending after lone ESC")
	}
	evs := p.FlushIdle()
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %#v", evs)
	}
	k, ok := evs[0].(KeyEvent)
	if !ok || k.Code != KeyEscape || !k.Press {
		t.Fatalf("got %#v", evs[0])
	}
	if p.Pending() {
		t.Fatal("buffer should drain after flush")
	}
	// The next key must not glue onto the flushed Esc as an Alt+rune.
	evs = p.Feed([]byte("a"))
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	if k = evs[0].(KeyEvent); k.Rune != 'a' || k.Mods.Has(ModAlt) {
		t.Fatalf("got %#v", k)
	}
}

func TestParserPartialSequenceSurvivesIdle(t *testing.T) {
	p := NewParser()
	p.Feed([]byte{0x1b, '['})
	if evs := p.FlushIdle(); len(evs) != 0 {
		t.Fatalf("partial CSI is not a key, got %#v", evs)
	}
	if !p.Pending() {
		t.Fatal("partial sequence must stay buffered")
	}
	evs := p.Feed([]byte("A"))
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	if k := evs[0].(KeyEvent); k.Code != KeyUp {
		t.Fatalf("expected Up after completion, got %#v", k)
	}
}

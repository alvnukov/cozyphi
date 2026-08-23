package input

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	bracketedPasteStart = []byte("\x1b[200~")
	bracketedPasteEnd   = []byte("\x1b[201~")
)

// Parser converts raw TTY bytes into Events.
type Parser struct {
	buf      []byte
	inPaste  bool
	pasteBuf []byte
}

// NewParser creates an empty parser.
func NewParser() *Parser {
	return &Parser{}
}

// Reset clears incomplete sequence state.
func (p *Parser) Reset() {
	p.buf = p.buf[:0]
	p.inPaste = false
	p.pasteBuf = p.pasteBuf[:0]
}

// Feed appends bytes and returns complete events. Incomplete sequences remain buffered.
func (p *Parser) Feed(data []byte) []Event {
	p.buf = append(p.buf, data...)
	var events []Event
	for len(p.buf) > 0 {
		var n int
		var ev Event
		var ok bool
		if p.inPaste {
			n, ev, ok = p.consumePaste(p.buf)
		} else {
			n, ev, ok = p.parseOne(p.buf)
		}
		if !ok {
			break
		}
		p.buf = p.buf[n:]
		if ev != nil {
			events = append(events, ev)
		}
	}
	// Cap buffer growth on garbage (not while assembling a paste).
	if !p.inPaste && len(p.buf) > 4096 {
		p.buf = p.buf[len(p.buf)-1024:]
	}
	if p.inPaste && len(p.pasteBuf) > 1<<20 {
		// Bound paste size; force-end to avoid unbounded growth.
		text := normalizePaste(string(p.pasteBuf))
		p.pasteBuf = p.pasteBuf[:0]
		p.inPaste = false
		events = append(events, PasteEvent{Text: text})
	}
	return events
}

func (p *Parser) parseOne(b []byte) (consumed int, ev Event, ok bool) {
	if len(b) == 0 {
		return 0, nil, false
	}
	// Bracketed paste start: ESC [ 200 ~ (must win over generic CSI).
	if isBracketedPasteStart(b) {
		if len(b) < len(bracketedPasteStart) {
			return 0, nil, false
		}
		p.inPaste = true
		p.pasteBuf = p.pasteBuf[:0]
		return len(bracketedPasteStart), nil, true
	}
	// ESC sequences
	if b[0] == 0x1b {
		return parseESC(b)
	}
	// C0 controls
	switch b[0] {
	case 0x03: // Ctrl+C
		return 1, KeyEvent{Code: KeyRune, Rune: 'c', Mods: ModCtrl, Press: true}, true
	case 0x0d, 0x0a:
		return 1, KeyEvent{Code: KeyEnter, Press: true}, true
	case 0x09:
		return 1, KeyEvent{Code: KeyTab, Press: true}, true
	case 0x7f, 0x08:
		return 1, KeyEvent{Code: KeyBackspace, Press: true}, true
	case 0x00:
		return 1, nil, true
	}
	if b[0] < 0x20 {
		// Other Ctrl+letter
		r := rune(b[0] + 0x60)
		return 1, KeyEvent{Code: KeyRune, Rune: r, Mods: ModCtrl, Press: true}, true
	}
	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size == 1 {
		return 1, nil, true
	}
	return size, KeyEvent{Code: KeyRune, Rune: r, Text: string(b[:size]), Press: true}, true
}

// consumePaste buffers raw paste bytes until ESC [ 201 ~ (main.js handleBracketedPaste).
func (p *Parser) consumePaste(b []byte) (consumed int, ev Event, ok bool) {
	if idx := bytes.Index(b, bracketedPasteEnd); idx >= 0 {
		p.pasteBuf = append(p.pasteBuf, b[:idx]...)
		text := normalizePaste(string(p.pasteBuf))
		p.pasteBuf = p.pasteBuf[:0]
		p.inPaste = false
		return idx + len(bracketedPasteEnd), PasteEvent{Text: text}, true
	}
	// Hold back a suffix that might be a partial end sequence.
	hold := partialSuffixLen(b, bracketedPasteEnd)
	if hold == len(b) {
		return 0, nil, false
	}
	if hold > 0 {
		p.pasteBuf = append(p.pasteBuf, b[:len(b)-hold]...)
		return len(b) - hold, nil, true
	}
	p.pasteBuf = append(p.pasteBuf, b...)
	return len(b), nil, true
}

func isBracketedPasteStart(b []byte) bool {
	if len(b) == 0 || b[0] != 0x1b {
		return false
	}
	if len(b) >= len(bracketedPasteStart) {
		return bytes.HasPrefix(b, bracketedPasteStart)
	}
	return bytes.HasPrefix(bracketedPasteStart, b)
}

func partialSuffixLen(b, seq []byte) int {
	max := len(seq) - 1
	if max > len(b) {
		max = len(b)
	}
	for n := max; n >= 1; n-- {
		if bytes.Equal(b[len(b)-n:], seq[:n]) {
			return n
		}
	}
	return 0
}

func normalizePaste(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func parseESC(b []byte) (int, Event, bool) {
	if len(b) < 2 {
		return 0, nil, false
	}
	switch b[1] {
	case '[':
		// X10 mouse (1000/1002 without SGR 1006): ESC [ M Cb Cx Cy.
		// CSI would treat the 'M' as a final byte with empty params, consume
		// only ESC [ M, and leak Cb/Cx/Cy as KeyRunes into the focused field
		// (wheel up → '`' / wheel down → 'a', then ';' and digits for coords).
		if len(b) >= 3 && b[2] == 'M' {
			if len(b) < 6 {
				return 0, nil, false
			}
			return 6, parseX10Mouse(b[3], b[4], b[5]), true
		}
		return parseCSI(b)
	case 'O':
		if len(b) < 3 {
			return 0, nil, false
		}
		return parseSS3(b)
	case 'P': // DCS — skip until ST
		return skipUntilST(b)
	case '_': // APC
		return skipUntilST(b)
	case ']': // OSC
		return parseOSC(b)
	case 0x1b:
		return 1, KeyEvent{Code: KeyEscape, Press: true}, true
	default:
		// Alt+key
		if b[1] >= 0x20 {
			r, size := utf8.DecodeRune(b[1:])
			if r == utf8.RuneError && size == 1 {
				return 2, KeyEvent{Code: KeyEscape, Press: true}, true
			}
			return 1 + size, KeyEvent{Code: KeyRune, Rune: r, Text: string(b[1 : 1+size]), Mods: ModAlt, Press: true}, true
		}
		return 1, KeyEvent{Code: KeyEscape, Press: true}, true
	}
}

func skipUntilST(b []byte) (int, Event, bool) {
	// ESC P ... ESC \  or BEL
	for i := 2; i < len(b); i++ {
		if b[i] == 0x07 {
			return i + 1, nil, true
		}
		if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
			return i + 2, nil, true
		}
	}
	return 0, nil, false
}

func parseOSC(b []byte) (int, Event, bool) {
	for i := 2; i < len(b); i++ {
		if b[i] == 0x07 {
			return i + 1, nil, true
		}
		if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
			return i + 2, nil, true
		}
	}
	return 0, nil, false
}

func parseSS3(b []byte) (int, Event, bool) {
	// ESC O A/B/C/D etc
	var code KeyCode
	switch b[2] {
	case 'A':
		code = KeyUp
	case 'B':
		code = KeyDown
	case 'C':
		code = KeyRight
	case 'D':
		code = KeyLeft
	case 'H':
		code = KeyHome
	case 'F':
		code = KeyEnd
	case 'P':
		code = KeyF1
	case 'Q':
		code = KeyF2
	case 'R':
		code = KeyF3
	case 'S':
		code = KeyF4
	default:
		return 3, nil, true
	}
	return 3, KeyEvent{Code: code, Press: true}, true
}

func parseCSI(b []byte) (int, Event, bool) {
	// Find final byte (0x40-0x7E) after optional intermediates.
	i := 2
	for i < len(b) {
		c := b[i]
		if c >= 0x40 && c <= 0x7e {
			seq := b[2:i]
			final := c
			consumed := i + 1
			ev := dispatchCSI(seq, final, b[:consumed])
			return consumed, ev, true
		}
		i++
	}
	return 0, nil, false
}

func dispatchCSI(params []byte, final byte, raw []byte) Event {
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		mods, press := parseModsAndPress(params)
		var code KeyCode
		switch final {
		case 'A':
			code = KeyUp
		case 'B':
			code = KeyDown
		case 'C':
			code = KeyRight
		case 'D':
			code = KeyLeft
		case 'H':
			code = KeyHome
		case 'F':
			code = KeyEnd
		}
		return KeyEvent{Code: code, Mods: mods, Press: press}
	case '~':
		return parseTildeKey(params)
	case 'u':
		// Kitty keyboard query reply: CSI ? <flags> u (libvaxis / kitty protocol).
		if len(raw) > 2 && raw[2] == '?' {
			return CapEvent{Kind: CapKittyKB, Data: string(raw)}
		}
		// Kitty keyboard / CSI-u: ESC [ code ; mods u
		return parseCSIu(params)
	case 'c':
		// DA1 response ESC [ ? ... c  or ESC [ ... c
		return CapEvent{Kind: CapDA1, Data: string(raw)}
	case 'm', 'M':
		// SGR mouse: ESC [ < btn ; x ; y M/m
		return parseSGRMouse(params, final)
	case 'I':
		return FocusEvent{Focused: true}
	case 'O':
		// Could be focus out ESC [ O — but also SS3. Here inside CSI.
		return FocusEvent{Focused: false}
	case 't':
		// In-band resize: ESC [ 48 ; height ; width ; ... t
		return parseInBandResize(params)
	case 'n':
		return nil // DSR ignore
	case 'p':
		// DECRQSS / DECRQM response
		return CapEvent{Kind: CapDECRQM, Data: string(raw)}
	case 'q':
		if bytes.Contains(raw, []byte{'>'}) {
			return CapEvent{Kind: CapXTVersion, Data: string(raw)}
		}
		return CapEvent{Kind: CapKittyKB, Data: string(raw)}
	case 'Z':
		return KeyEvent{Code: KeyTab, Mods: ModShift, Press: true}
	}
	return nil
}

// parseModsAndPress reads CSI params of the form [code ;] mods [: event_type]
// used by cursor keys (CSI A/B/…) and ~-keys. A lone numeric parameter is the
// leading cursor code (e.g. CSI 1 A), not modifiers.
// Kitty reportEventTypes uses event_type 3 for release (Press=false); 1=press, 2=repeat.
func parseModsAndPress(params []byte) (mods Modifiers, press bool) {
	press = true
	if len(params) == 0 {
		return 0, true
	}
	fields := bytes.Split(params, []byte{';'})
	if len(fields) == 1 && !bytes.Contains(fields[0], []byte{':'}) {
		return 0, true
	}
	return parseModField(fields[len(fields)-1])
}

// parseModField parses "mods" or "mods:event_type".
func parseModField(modField []byte) (mods Modifiers, press bool) {
	press = true
	sub := bytes.Split(modField, []byte{':'})
	n, _ := strconv.Atoi(string(sub[0]))
	mods = modsFromCSI(n)
	if len(sub) >= 2 {
		et, _ := strconv.Atoi(string(sub[1]))
		if et == 3 {
			press = false
		}
	}
	return mods, press
}

func modsFromCSI(n int) Modifiers {
	if n <= 1 {
		return 0
	}
	n-- // CSI mods are 1-based mask+1
	var m Modifiers
	if n&1 != 0 {
		m |= ModShift
	}
	if n&2 != 0 {
		m |= ModAlt
	}
	if n&4 != 0 {
		m |= ModCtrl
	}
	if n&8 != 0 {
		m |= ModSuper
	}
	return m
}

func parseTildeKey(params []byte) Event {
	parts := splitParams(params)
	if len(parts) == 0 {
		return nil
	}
	// xterm modifyOtherKeys mode 2: CSI 27 ; mods ; keycode ~
	// e.g. Shift+Enter → ESC [ 27 ; 2 ; 13 ~
	if parts[0] == 27 && len(parts) >= 3 {
		mods := modsFromCSI(parts[1])
		ev := keyFromCodepoint(parts[2], mods)
		ev.Press = true
		return ev
	}
	code := tildeKeyCode(parts[0])
	if code == KeyNone {
		return nil
	}
	mods, press := parseModsAndPress(params)
	return KeyEvent{Code: code, Mods: mods, Press: press}
}

func tildeKeyCode(n int) KeyCode {
	switch n {
	case 1, 7:
		return KeyHome
	case 2:
		return KeyInsert
	case 3:
		return KeyDelete
	case 4, 8:
		return KeyEnd
	case 5:
		return KeyPageUp
	case 6:
		return KeyPageDown
	case 11:
		return KeyF1
	case 12:
		return KeyF2
	case 13:
		return KeyF3
	case 14:
		return KeyF4
	case 15:
		return KeyF5
	case 17:
		return KeyF6
	case 18:
		return KeyF7
	case 19:
		return KeyF8
	case 20:
		return KeyF9
	case 21:
		return KeyF10
	case 23:
		return KeyF11
	case 24:
		return KeyF12
	case 200:
		// bracketed paste start — handled specially if we buffered; treat as none
		return KeyNone
	case 201:
		return KeyNone
	default:
		return KeyNone
	}
}

func parseCSIu(params []byte) Event {
	fields := bytes.Split(params, []byte{';'})
	if len(fields) == 0 || len(fields[0]) == 0 {
		return nil
	}
	codepoint, _ := strconv.Atoi(string(fields[0]))
	var mods Modifiers
	press := true
	if len(fields) >= 2 {
		mods, press = parseModField(fields[1])
	}
	ev := keyFromCodepoint(codepoint, mods)
	ev.Press = press
	return ev
}

func keyFromCodepoint(codepoint int, mods Modifiers) KeyEvent {
	switch codepoint {
	case 27:
		return KeyEvent{Code: KeyEscape, Mods: mods}
	case 13:
		return KeyEvent{Code: KeyEnter, Mods: mods}
	case 9:
		return KeyEvent{Code: KeyTab, Mods: mods}
	case 127:
		return KeyEvent{Code: KeyBackspace, Mods: mods}
	default:
		r := rune(codepoint)
		return KeyEvent{Code: KeyRune, Rune: r, Text: string(r), Mods: mods}
	}
}

func parseSGRMouse(params []byte, final byte) Event {
	// ESC [ < b ; x ; y M/m  (also urxvt 1015 without '<')
	p := params
	if len(p) > 0 && p[0] == '<' {
		p = p[1:]
	}
	parts := splitParams(p)
	if len(parts) < 3 {
		return nil
	}
	ev := mouseFromButton(parts[0], parts[1]-1, parts[2]-1)
	// Wheel reports always use Press (same as prior SGR path).
	if final == 'm' && ev.Button != MouseWheelUp && ev.Button != MouseWheelDown {
		ev.Action = MouseRelease
	}
	return ev
}

// parseX10Mouse decodes legacy mouse bytes (each is value+32; x/y are 1-based).
func parseX10Mouse(cb, cx, cy byte) Event {
	return mouseFromButton(int(cb)-32, int(cx)-33, int(cy)-33)
}

func mouseFromButton(btn, x, y int) MouseEvent {
	ev := MouseEvent{X: x, Y: y, Action: MousePress}
	motion := btn&32 != 0
	if motion {
		ev.Action = MouseMotion
	}
	if btn&4 != 0 {
		ev.Mods |= ModShift
	}
	if btn&8 != 0 {
		ev.Mods |= ModAlt
	}
	if btn&16 != 0 {
		ev.Mods |= ModCtrl
	}
	b := btn & 0b11000011
	switch b {
	case 0:
		ev.Button = MouseLeft
	case 1:
		ev.Button = MouseMiddle
	case 2:
		ev.Button = MouseRight
	case 64:
		ev.Button = MouseWheelUp
		ev.Action = MousePress
		ev.Wheel = 1
	case 65:
		ev.Button = MouseWheelDown
		ev.Action = MousePress
		ev.Wheel = 1
	default:
		if motion {
			ev.Button = MouseNone
		}
	}
	if motion && ev.Button != MouseNone && ev.Button != MouseWheelUp && ev.Button != MouseWheelDown {
		ev.Action = MouseDrag
	}
	return ev
}

func parseInBandResize(params []byte) Event {
	parts := splitParams(params)
	// 48 ; rows ; cols ; height_px ; width_px
	if len(parts) < 3 || parts[0] != 48 {
		return nil
	}
	return ResizeEvent{Rows: parts[1], Cols: parts[2]}
}

func splitParams(params []byte) []int {
	if len(params) == 0 {
		return nil
	}
	// Strip leading '<' or '?' or '>'
	for len(params) > 0 && (params[0] == '<' || params[0] == '?' || params[0] == '>') {
		params = params[1:]
	}
	var out []int
	start := 0
	for i := 0; i <= len(params); i++ {
		if i == len(params) || params[i] == ';' || params[i] == ':' {
			if i > start {
				n, _ := strconv.Atoi(string(params[start:i]))
				out = append(out, n)
			} else {
				out = append(out, 0)
			}
			start = i + 1
		}
	}
	return out
}

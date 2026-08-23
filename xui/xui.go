package xui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/xui/input"
	"github.com/pulseaiclub/xui/render"
	"github.com/pulseaiclub/xui/screen"
	"github.com/pulseaiclub/xui/term"
)

// Options configures XUI.
type Options struct {
	Mouse          bool
	BracketedPaste bool
}

// XUI is the core TUI engine: screen, capabilities, and render.
type XUI struct {
	tty      term.TTY
	screen   *screen.Screen
	renderer *render.Renderer
	caps     render.Caps
	opts     Options

	altScreen   bool
	kittyPushed bool
	mu          sync.Mutex

	queryDone chan struct{}
	winchCh   chan os.Signal
	stopWinch chan struct{}
}

// New opens the TTY and creates a XUI instance.
func New(opts Options) (*XUI, error) {
	tty, err := term.OpenTTY()
	if err != nil {
		return nil, err
	}
	if err := tty.MakeRaw(); err != nil {
		_ = tty.Close()
		return nil, fmt.Errorf("xui: make raw: %w", err)
	}
	cols, rows, _ := tty.Size()
	vx := &XUI{
		tty:       tty,
		screen:    screen.NewScreen(cols, rows),
		renderer:  render.NewRenderer(),
		opts:      opts,
		queryDone: make(chan struct{}),
		stopWinch: make(chan struct{}),
	}
	vx.caps.RGB = true
	vx.renderer.UpdateCaps(vx.caps)
	return vx, nil
}

// Close restores the terminal and releases resources.
func (vx *XUI) Close() error {
	vx.stopWinchSignals()
	vx.mu.Lock()
	defer vx.mu.Unlock()
	var b strings.Builder
	if vx.opts.Mouse {
		b.WriteString(render.SeqMouseReset)
	}
	if vx.opts.BracketedPaste {
		b.WriteString(render.SeqBracketedPasteReset)
	}
	if vx.kittyPushed {
		b.WriteString(render.SeqKittyKBPop)
		vx.kittyPushed = false
	}
	b.WriteString(render.SeqModifyOtherKeysReset)
	if vx.caps.Unicode {
		b.WriteString(render.SeqUnicodeReset)
	}
	if vx.caps.InBandResize {
		b.WriteString(render.SeqInBandResizeReset)
	}
	if vx.altScreen {
		b.WriteString(render.ExitAltScreenSeq())
		vx.altScreen = false
	} else {
		b.WriteString(render.SeqSGRReset)
		b.WriteString(render.SeqShowCursor)
	}
	_, _ = vx.tty.Write([]byte(b.String()))
	return vx.tty.Close()
}

// Caps returns the current capability set.
func (vx *XUI) Caps() render.Caps {
	vx.mu.Lock()
	defer vx.mu.Unlock()
	return vx.caps
}

// Screen returns the underlying screen.
func (vx *XUI) Screen() *screen.Screen { return vx.screen }

// Window returns a full-screen Window for drawing.
func (vx *XUI) Window() screen.Window { return screen.NewWindow(vx.screen) }

// EnterAltScreen switches to the alternate screen buffer.
func (vx *XUI) EnterAltScreen() error {
	vx.mu.Lock()
	defer vx.mu.Unlock()
	if vx.altScreen {
		return nil
	}
	_, err := vx.tty.Write([]byte(render.EnterAltScreenSeq()))
	if err != nil {
		return err
	}
	vx.altScreen = true
	vx.renderer.ResetState()
	vx.screen.MarkRefresh()
	return nil
}

// ExitAltScreen leaves the alternate screen buffer.
func (vx *XUI) ExitAltScreen() error {
	vx.mu.Lock()
	defer vx.mu.Unlock()
	if !vx.altScreen {
		return nil
	}
	_, err := vx.tty.Write([]byte(render.ExitAltScreenSeq()))
	vx.altScreen = false
	vx.renderer.ResetState()
	return err
}

// EnableMouse turns on mouse tracking.
func (vx *XUI) EnableMouse() error {
	vx.opts.Mouse = true
	_, err := vx.tty.Write([]byte(render.SeqMouseSet))
	return err
}

// EnableBracketedPaste turns on bracketed paste.
func (vx *XUI) EnableBracketedPaste() error {
	vx.opts.BracketedPaste = true
	_, err := vx.tty.Write([]byte(render.SeqBracketedPasteSet))
	return err
}

// QueryTerminal sends capability probes and waits up to timeout for DA1.
func (vx *XUI) QueryTerminal(timeout time.Duration) {
	// Match libvaxis: probe features first, Primary DA last so its reply
	// arrives after Kitty/DECRQM responses and we can enable what we detected.
	queries := render.SeqXTVersion +
		render.SeqKittyKBQuery +
		render.SeqDECRQMSync +
		render.SeqDECRQMUnicode +
		render.SeqPrimaryDA
	_, _ = vx.tty.Write([]byte(queries))

	vx.mu.Lock()
	vx.queryDone = make(chan struct{})
	done := vx.queryDone
	vx.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	vx.mu.Lock()
	vx.renderer.UpdateCaps(vx.caps)
	vx.mu.Unlock()

	vx.enableDetectedFeatures()
}

// enableDetectedFeatures turns on Kitty keyboard / unicode / mouse / paste
// based on caps (and always enables modifyOtherKeys as a Shift+Enter fallback).
func (vx *XUI) enableDetectedFeatures() {
	vx.mu.Lock()
	caps := vx.caps
	pushKitty := caps.KittyKeyboard && !vx.kittyPushed
	if pushKitty {
		vx.kittyPushed = true
	}
	vx.mu.Unlock()

	var enable strings.Builder
	if caps.Unicode {
		enable.WriteString(render.SeqUnicodeSet)
	}
	if pushKitty {
		enable.WriteString(render.SeqKittyKBPush)
	}
	// Enables modifyOtherKeys mode 2 so Shift+Enter arrives as CSI 27;2;13~
	// when Kitty keyboard is unavailable (e.g. some tmux setups).
	enable.WriteString(render.SeqModifyOtherKeysSet)
	if s := enable.String(); s != "" {
		_, _ = vx.tty.Write([]byte(s))
	}
	if vx.opts.Mouse {
		_ = vx.EnableMouse()
	}
	if vx.opts.BracketedPaste {
		_ = vx.EnableBracketedPaste()
	}
}

func (vx *XUI) applyCap(e input.CapEvent) {
	var pushKitty bool
	vx.mu.Lock()
	switch e.Kind {
	case input.CapDA1:
		vx.caps.RGB = true
		select {
		case <-vx.queryDone:
		default:
			close(vx.queryDone)
		}
	case input.CapKittyKB:
		vx.caps.KittyKeyboard = true
		// Late reply after QueryTerminal finished: push now (once).
		if vx.queryDone != nil {
			select {
			case <-vx.queryDone:
				if !vx.kittyPushed {
					vx.kittyPushed = true
					pushKitty = true
				}
			default:
				// Still querying; enableDetectedFeatures will push.
			}
		}
	case input.CapDECRQM:
		if strings.Contains(e.Data, "?2026") && (strings.Contains(e.Data, ";1$") || strings.Contains(e.Data, ";2$")) {
			vx.caps.SyncOutput = true
		}
		if strings.Contains(e.Data, "?2027") && (strings.Contains(e.Data, ";1$") || strings.Contains(e.Data, ";2$")) {
			vx.caps.Unicode = true
		}
	case input.CapXTVersion:
		vx.caps.RGB = true
	}
	vx.renderer.UpdateCaps(vx.caps)
	vx.mu.Unlock()

	if pushKitty {
		_, _ = vx.tty.Write([]byte(render.SeqKittyKBPush))
	}
}

// Resize updates the screen. Must be called on the main goroutine.
// Always clears the TTY and marks a full refresh: duplicate SIGWINCH with the
// same cols/rows is common (e.g. Ghostty drag end), and clearing without
// MarkRefresh leaves Diff empty → blank screen until the next forced paint.
func (vx *XUI) Resize(cols, rows int) {
	vx.mu.Lock()
	defer vx.mu.Unlock()
	vx.screen.Resize(cols, rows)
	vx.renderer.ResetState()
	_, _ = vx.tty.Write([]byte(render.SeqClearScreen + render.SeqHome))
	vx.screen.MarkRefresh()
}

// ResizeToTTY queries the TTY size and resizes.
func (vx *XUI) ResizeToTTY() {
	cols, rows, err := vx.tty.Size()
	if err != nil {
		return
	}
	vx.Resize(cols, rows)
}

// Render diffs the screen and writes ANSI to the TTY.
func (vx *XUI) Render() error {
	vx.mu.Lock()
	defer vx.mu.Unlock()
	dirty := vx.screen.Diff()
	cx, cy, vis, shape := vx.screen.Cursor()
	_, err := vx.renderer.RenderDiff(vx.tty, dirty, cx, cy, vis, shape)
	if err != nil {
		return err
	}
	vx.screen.Present()
	return nil
}

// QueueRefresh forces a full redraw on the next Render.
func (vx *XUI) QueueRefresh() { vx.screen.MarkRefresh() }

// WriteRaw writes bytes directly to the TTY.
func (vx *XUI) WriteRaw(p []byte) (int, error) { return vx.tty.Write(p) }

// NotifyWinsize starts delivering ResizeEvents to loop on SIGWINCH (unix).
func (vx *XUI) NotifyWinsize(loop *Loop) { vx.startWinchSignals(loop) }

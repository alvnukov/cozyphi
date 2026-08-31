package browse

import "github.com/pulseaiclub/xui"

// Confirm is one armed y/n question in a pane footer. Arming a new
// question replaces the old, so a double y can never fire two different
// actions.
type Confirm struct {
	label  string
	accept func()
}

// Arm poses the question. label names the exact target — `Delete step 2,
// "wire the pane"?` — and accept runs when the user presses y.
func (c *Confirm) Arm(label string, accept func()) {
	c.label, c.accept = label, accept
}

// Disarm withdraws the question without firing it.
func (c *Confirm) Disarm() { c.label, c.accept = "", nil }

// Armed reports whether a question is waiting for y or n.
func (c *Confirm) Armed() bool { return c.accept != nil }

// Label is the question to render, empty when nothing is armed. The
// footer adds its own "(y/n)" tail.
func (c *Confirm) Label() string { return c.label }

// Key answers one pressed key while armed: y fires and disarms, n and Esc
// disarm, and any other key disarms without firing — acting elsewhere
// withdraws the question. consumed reports that the key was an answer;
// otherwise the caller still owns the event, now with nothing armed.
func (c *Confirm) Key(e xui.KeyEvent) (consumed bool) {
	if !c.Armed() {
		return false
	}
	if e.Code == xui.KeyEscape {
		c.Disarm()
		return true
	}
	if e.Code == xui.KeyRune && e.Mods == 0 {
		switch e.HotkeyRune() {
		case 'y':
			accept := c.accept
			c.Disarm()
			accept()
			return true
		case 'n':
			c.Disarm()
			return true
		}
	}
	c.Disarm()
	return false
}

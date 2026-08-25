package components

import "github.com/pulseaiclub/xui"

// CopyKeyAcceptor is implemented by focused text widgets that claim a copy
// chord before the runtime applies its own binding. The App consults it on
// the built-in Ctrl+C quit path: an active selection turns Ctrl+C into a
// copy, and only an unclaimed press exits the app.
type CopyKeyAcceptor interface {
	AcceptCopyKey(e xui.KeyEvent) bool
}

package notify

import "github.com/alvnukov/cozyphi/internal/diag"

// ObserveConfig projects the notifications section into the shape the
// diagnostics registry reports. Both values are already decoded, so what is
// reported is what the notifier would be built from rather than the raw keys
// somebody wrote.
//
// It observes and returns. It reads no configuration file and builds no
// notifier.
func ObserveConfig(mode Mode, sound string) diag.NotifyConfigFacts {
	return diag.NotifyConfigFacts{Known: true, Mode: mode.String(), Sound: sound}
}

// Observe projects the live notifier into the same shape: what it would do
// and what it has already found out. Every value comes from an atomic, so it
// is safe from any goroutine, and a nil notifier reports absence rather than
// a mode nobody set.
//
// Nothing about a notification's content travels — not a title, not a body,
// not the error a failed sender returned. Only whether one would arrive.
//
// It observes and returns. It sends nothing, reconfigures nothing and does
// not resurrect a sender that already failed.
func (n *Notifier) Observe() diag.NotifierFacts {
	if n == nil {
		return diag.NotifierFacts{}
	}
	return diag.NotifierFacts{
		Known:        true,
		Mode:         n.currentMode().String(),
		Sound:        n.Sound(),
		Broken:       n.broken.Load(),
		FocusTrusted: n.focusTrusted.Load(),
		Focused:      n.focused.Load(),
	}
}

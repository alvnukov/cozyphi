package usagepane

import (
	"errors"
	"fmt"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type resetState struct {
	busy, invalidated       bool
	armed, presented        *provider.QuotaResetTarget
	note                    string
	button, confirm, cancel resetHit
}

type resetHit struct{ x, y, width int }

func (h resetHit) contains(e xui.MouseEvent) bool {
	return h.width > 0 && e.Y == h.y && e.X >= h.x && e.X < h.x+h.width
}

// InvalidateReset withdraws consent and the old quota capability, but never
// clears an in-flight mutation. The editor calls it on session/model changes.
func (p *Pane) InvalidateReset() {
	p.cancelReset()
	p.quota.Snapshot.ResetTarget = nil
	p.reset.invalidated = true
}

func (p *Pane) cancelReset() {
	p.reset.armed, p.reset.presented = nil, nil
	p.reset.button, p.reset.confirm, p.reset.cancel = resetHit{}, resetHit{}, resetHit{}
}

// ApplyReset accepts lifecycle messages even while hidden. Results never trigger
// another mutation or fetch; a later fetch failure cannot erase a reset outcome.
func (p *Pane) ApplyReset(msg controller.UsageResetMsg) {
	p.InvalidateReset()
	p.reset.busy = msg.InFlight
	if msg.InFlight {
		p.reset.note = ""
		return
	}
	switch {
	case errors.Is(msg.Err, provider.ErrQuotaResetUnknown):
		p.reset.note = "Reset outcome unknown; check Codex before trying again."
	case errors.Is(msg.Err, provider.ErrQuotaResetStale):
		p.reset.note = "Reset target changed; press r to refresh before trying again."
	case msg.Err != nil:
		// Only known display-safe errors cross this mutation boundary.
		p.reset.note = "Reset failed; press r to refresh before trying again."
	case msg.Result.Code == provider.QuotaResetDone:
		p.reset.note = fmt.Sprintf(
			"Reset complete: %d windows reset.",
			msg.Result.WindowsReset,
		)
	case msg.Result.Code == provider.QuotaResetNothing:
		p.reset.note = "No limits needed resetting."
	case msg.Result.Code == provider.QuotaResetNoCredit:
		p.reset.note = "No reset credits available."
	case msg.Result.Code == provider.QuotaResetAlreadyRedeemed:
		p.reset.note = "Reset credit already redeemed."
	default:
		p.reset.note = "Reset outcome unknown; check Codex before trying again."
	}
}

func (p *Pane) resetDisabled() string {
	switch {
	case p.reset.busy:
		return "Reset in progress; please wait."
	case p.onReset == nil:
		return "Open /usage to reset limits."
	case p.sessionStats == nil || p.session.ProviderID == "":
		return "Not connected; open /connect."
	case p.loading:
		return "Waiting for subscription usage."
	case p.reset.invalidated:
		return "Press r to refresh before resetting."
	case p.quota.Unsupported:
		return "Reset unavailable for this provider or connection."
	case p.quota.Err != nil:
		return "Quota unavailable; check /connect and press r to refresh."
	case !p.quota.Snapshot.Reset.Supported:
		return "Reset credit count unknown; press r to refresh."
	case p.quota.Snapshot.Reset.Available <= 0:
		return "No reset credits available."
	case p.quota.Snapshot.ResetTarget == nil:
		return "Reset unavailable; connect a supported OAuth account."
	default:
		return ""
	}
}

func (p *Pane) armReset() {
	p.pullSession()
	if p.resetDisabled() != "" || p.reset.armed != nil {
		return
	}
	p.reset.armed = p.quota.Snapshot.ResetTarget
	p.reset.button = resetHit{}
}

func (p *Pane) confirmReset() {
	p.pullSession()
	target := p.reset.armed
	if target == nil || target != p.reset.presented || target != p.quota.Snapshot.ResetTarget ||
		p.resetDisabled() != "" {
		p.cancelReset()
		return
	}
	// Disarm and claim the busy slot before calling the shell, including when
	// the callback synchronously delivers another event or reset result.
	p.InvalidateReset()
	p.reset.busy = true
	p.reset.note = ""
	p.onReset(target)
}

func (p *Pane) handleResetKey(e xui.KeyEvent) bool {
	if p.reset.armed != nil {
		if e.Code == xui.KeyEscape || (e.Code == xui.KeyRune && e.Rune == 'n') {
			p.cancelReset()
			return true
		}
		if e.Code == xui.KeyRune && e.Rune == 'y' {
			p.confirmReset()
			return true
		}
	}
	if e.Code == xui.KeyRune && e.Rune == 'x' {
		p.armReset()
		return true
	}
	return false
}

func (p *Pane) handleResetMouse(e xui.MouseEvent) {
	if e.Action != xui.MousePress || e.Button != xui.MouseLeft {
		return
	}
	switch {
	case p.reset.armed != nil && p.reset.confirm.contains(e):
		p.confirmReset()
	case p.reset.armed != nil && p.reset.cancel.contains(e):
		p.cancelReset()
	case p.reset.armed == nil && p.reset.button.contains(e):
		p.armReset()
	}
}

// Actions live only in the standalone viewport, never in the dashboard Report.
const resetWarning = "Spend one reset credit to reset eligible usage limits?"

func (p *Pane) drawReset(s components.Surface, method xui.WidthMethod, w, h int) {
	p.reset.presented = nil
	p.reset.button, p.reset.confirm, p.reset.cancel = resetHit{}, resetHit{}, resetHit{}
	if p.onReset == nil || h < 6 {
		return
	}
	print := func(y int, text string) {
		s.Print(1, y, layout.TruncateToWidth(text, max(0, w-2), method), p.theme.Muted, method)
	}
	if w < xui.StringWidth(resetWarning, method)+2 {
		print(h-3, "Enlarge pane to reset limits.")
		return
	}
	button := func(x, y int, label string) resetHit {
		if x+len(label) >= w {
			return resetHit{}
		}
		s.Print(x, y, label, p.theme.Warning, method)
		return resetHit{x: x, y: y, width: len(label)}
	}
	if p.reset.armed != nil {
		print(h-3, resetWarning)
		// Never reuse the Reset row for Confirm: two presses at the original
		// button's coordinates must not spend a credit, even across redraws.
		p.reset.confirm = button(1, h-2, "[Confirm y]")
		p.reset.cancel = button(14, h-2, "[Cancel n/Esc]")
		// Consent is usable only after the full cost warning and controls have
		// been rendered for this intent. Queued x/y before a frame sends nothing.
		p.reset.presented = p.reset.armed
	} else if reason := p.resetDisabled(); reason != "" {
		print(h-3, "[Reset limit x — disabled]")
		print(h-2, reason)
	} else {
		p.reset.button = button(1, h-3, "[Reset limit x]")
	}
	print(h-1, p.reset.note)
}

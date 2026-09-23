package composer

import (
	"strings"

	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/voice"
)

const asidePlaceholder = "side question — answer stays out of context"

// SetAsideSubmit wires the one-shot side-question path. Returning false keeps
// the draft and anchor in place so a refused request can be retried.
func (c *ComposerPane) SetAsideSubmit(submit func(anchor, question string) bool) {
	if c != nil {
		c.asideSubmit = submit
		if submit == nil {
			c.Chat.OnLeadClick = nil
			c.Chat.LeadTooltip = nil
		} else {
			c.Chat.OnLeadClick = c.ToggleAside
			c.Chat.LeadTooltip = func() string {
				return "Ask a side question — answer stays out of context (" + keys.Label(keys.CmdAside) + ")"
			}
		}
	}
}

// EnterAside starts a side question, optionally about a transcript entry.
// It does not silently discard an image, a skill or a running voice dialog.
func (c *ComposerPane) EnterAside(anchor string) bool {
	if c == nil || c.asideSubmit == nil || c.voiceState != voice.StateIdle || c.bashActive ||
		strings.HasPrefix(strings.TrimSpace(c.Chat.Value), "!") ||
		len(c.attachedMedia) > 0 || len(c.Chat.PendingSkills) > 0 {
		return false
	}
	c.HideCompleters()
	c.asideActive = true
	c.asideAnchor = anchor
	c.applyPosture()
	c.FocusChat()
	return true
}

// ToggleAside toggles the current-context mode; clicking an anchored message
// instead calls EnterAside with that message's entry ID.
func (c *ComposerPane) ToggleAside() {
	if c == nil {
		return
	}
	if c.asideActive {
		c.LeaveAside()
	} else {
		c.EnterAside("")
	}
}

// LeaveAside restores the normal input posture while keeping the draft.
func (c *ComposerPane) LeaveAside() {
	if c == nil || !c.asideActive {
		return
	}
	c.asideActive = false
	c.asideAnchor = ""
	c.applyPosture()
}

// submitAside handles a chat Enter before the regular prompt bus sees it.
func (c *ComposerPane) submitAside(text string) {
	if strings.TrimSpace(text) == "" || c.asideSubmit == nil {
		return
	}
	if !c.asideSubmit(c.asideAnchor, text) {
		return
	}
	c.ClearInput()
	c.LeaveAside()
}

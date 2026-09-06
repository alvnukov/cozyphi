package sessions

import (
	"fmt"
	"strings"
	"unicode"
)

// SetIdentity binds presentation to registry membership, never to a selected controller.
// Notifier origin is optional so existing focus adapters keep their interface.
func (e *View) SetIdentity(number int, name string) {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
	e.lifetime.slot = name
	e.lifetime.identity = fmt.Sprintf("#%d %s", number, name)
	if n, ok := e.notifier.(interface{ SetOrigin(string) }); ok {
		n.SetOrigin(e.attentionOrigin(number, name))
	}
	if e.composer != nil {
		e.composer.Chat.SessionLabel = e.lifetime.identity
	}
}

// attentionOrigin names the session in OS notifications. The input line keeps
// the stable registry slot (see SetIdentity), but a notification lands on a
// desktop with no screen around it: the explicit session title — the model's
// set_title or the user's /rename — says what actually stopped, so it wins
// over the slot name. Job views keep the slot name, matching DisplayName.
func (e *View) attentionOrigin(number int, slot string) string {
	if e.ctrl != nil && e.ctrl.Assignment().JobID == "" {
		if title := e.ctrl.SessionName(); title != "" {
			return fmt.Sprintf("#%d %s", number, title)
		}
	}
	return fmt.Sprintf("#%d %s", number, slot)
}

func (e *View) recordAttention(text string) {
	if !e.Active() {
		e.lifetime.status.Attention = strings.TrimSpace(text)
		// Waiting/error marks carry attention; Unread counts completed turns only.
	}
}

func (e *View) acknowledgeViewed() {
	// Selection is not observation: a modal or a scrolled-up transcript must
	// retain the mark until the user actually sees the bottom of this view.
	if e.Active() && !e.modalActive() && e.transcript.AtBottom() {
		e.lifetime.status.Unread = 0
		e.lifetime.status.Attention = ""
	}
}

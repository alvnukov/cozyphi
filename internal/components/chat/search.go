package chat

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// search is ChatInput's reverse-i-search mode: bash's Ctrl+R, adapted so
// "execute" means sending the prompt. While active, typing edits the query —
// the draft stays untouched until a key accepts the current match into the
// buffer (Enter submits it too) or aborts back to the saved draft.
type search struct {
	query   string   // the live query; matches re-resolve on every edit
	matches []string // from History.Search, newest first
	index   int      // 0 = newest match

	// savedValue and savedCursor capture the draft on entry; abort gives
	// them back exactly, match discarded.
	savedValue  string
	savedCursor int

	active bool
}

// SearchActive reports whether reverse-i-search owns the composer.
func (c *ChatInput) SearchActive() bool { return c.search.active }

// BeginSearch enters reverse-i-search, saving the draft for a later abort.
// It reports false — and changes nothing — without a history store, so the
// chord stays unconsumed.
func (c *ChatInput) BeginSearch() bool {
	if c.History == nil {
		return false
	}
	c.ClearSelection() // a stale selection would tint the match preview
	c.search = search{active: true, savedValue: c.Value, savedCursor: c.Cursor}
	return true
}

// SearchOlder steps to the next older match — the repeated Ctrl+R. It
// reports false at the oldest match: there is nothing older to show.
func (c *ChatInput) SearchOlder() bool {
	if !c.search.active || c.search.index >= len(c.search.matches)-1 {
		return false
	}
	c.search.index++
	return true
}

// SearchNewer steps to the next newer match, Ctrl+S while searching. It
// floors at the newest match, the way bash does.
func (c *ChatInput) SearchNewer() bool {
	if !c.search.active || c.search.index == 0 {
		return false
	}
	c.search.index--
	return true
}

// SearchAbort is the Ctrl+G action: leave the mode and hand the draft back
// exactly as it was captured, match discarded. The buffer was never touched,
// so no change notification is owed.
func (c *ChatInput) SearchAbort() {
	if !c.search.active {
		return
	}
	c.Value = c.search.savedValue
	c.Cursor = c.search.savedCursor
	c.ClearSelection()
	c.search = search{}
}

// searchMatch returns the match the mode currently sits on.
func (c *ChatInput) searchMatch() (string, bool) {
	if !c.search.active || c.search.index >= len(c.search.matches) {
		return "", false
	}
	return c.search.matches[c.search.index], true
}

// searchType appends r to the query and re-resolves the matches, landing on
// the newest one.
func (c *ChatInput) searchType(r rune) {
	c.search.query += string(r)
	c.resolveSearch()
}

// searchBackspace drops the last rune of the query; on the empty query it is
// a no-op that keeps the mode.
func (c *ChatInput) searchBackspace() {
	if c.search.query == "" {
		return
	}
	_, size := utf8.DecodeLastRuneInString(c.search.query)
	c.search.query = c.search.query[:len(c.search.query)-size]
	c.resolveSearch()
}

func (c *ChatInput) resolveSearch() {
	if c.History == nil {
		c.search.matches = nil
		c.search.index = 0
		return
	}
	c.search.matches = c.History.Search(c.search.query)
	c.search.index = 0
}

// searchAccept leaves the mode writing the current match into the buffer,
// caret at its end — Enter, Esc, Tab and the navigation keys. Without a
// match it only leaves the mode: the draft already is the buffer.
func (c *ChatInput) searchAccept() {
	if m, ok := c.searchMatch(); ok {
		c.Value = m
		c.Cursor = len(m)
		c.ClearSelection()
		c.notifyChange()
	}
	c.search = search{}
}

// handleSearchKey serves one key while reverse-i-search is active. It
// returns false for the navigation keys (arrows, Home/End) after ending the
// mode, so the same press then moves the caret as usual — bash's behavior.
// Chords stay untouched: they fall through to the editor (and the pane
// ladder, which owns the Ctrl+R/Ctrl+S/Ctrl+G steps).
func (c *ChatInput) handleSearchKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	switch e.Code {
	case xui.KeyEnter:
		if e.Mods != 0 {
			return false // Shift/Ctrl/Alt+Enter keep their newline meaning
		}
		m, ok := c.searchMatch()
		if !ok {
			ctx.ConsumeAndRedraw() // nothing to send: stay in the mode
			return true
		}
		c.searchAccept()
		if c.OnSubmit != nil {
			c.OnSubmit(m)
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEscape:
		c.searchAccept()
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyBackspace:
		c.searchBackspace()
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyDelete:
		// Not a query key; consuming keeps the draft pristine instead of
		// deleting forward into a buffer the mode is only previewing.
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyTab:
		// Accept and swallow the key: the pane would otherwise read it as
		// the mode toggle.
		c.searchAccept()
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyUp, xui.KeyDown, xui.KeyLeft, xui.KeyRight, xui.KeyHome, xui.KeyEnd:
		c.searchAccept()
		return false
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) || e.Mods.Has(xui.ModSuper) {
			return false
		}
		if e.Rune >= 0x20 {
			c.searchType(e.Rune)
			ctx.ConsumeAndRedraw()
		}
		return true
	}
	return false
}

// searchMetaSpans builds the meta-row lead while the mode is on: the bash
// prompt "(reverse-i-search) 'query'" in the posture style, the match
// position muted after it, the model label kept as the row's tail.
func (c *ChatInput) searchMetaSpans(lead xui.Style, th components.Theme) []components.Span {
	spans := []components.Span{
		{Text: "(reverse-i-search) '", Style: lead},
		{Text: c.search.query, Style: lead},
		{Text: "'", Style: lead},
	}
	if n := len(c.search.matches); n > 0 {
		spans = append(spans,
			components.Span{Text: "  ", Style: th.Muted},
			components.Span{Text: fmt.Sprintf("%d/%d", c.search.index+1, n), Style: th.Muted},
		)
	}
	if c.ModelLabel != "" {
		spans = append(spans,
			components.Span{Text: " · ", Style: th.Muted},
			components.Span{Text: c.ModelLabel, Style: th.Foreground},
		)
	}
	return spans
}

// paintSearchHit tints the first case-insensitive occurrence of the query in
// the previewed match with the selection colors — the visual cue that the
// body is a search result, not the draft.
func (c *ChatInput) paintSearchHit(
	s *components.Surface,
	rows []visRow,
	match string,
	scroll, editorRows, editorTop, textX, w int,
	th components.Theme,
) {
	off := strings.Index(strings.ToLower(match), strings.ToLower(c.search.query))
	if off < 0 {
		return
	}
	selA, selB := off, off+len(c.search.query)
	for i := range editorRows {
		li := i + scroll
		if li < 0 || li >= len(rows) {
			continue
		}
		fromCol, toCol, ok := rowSelectionCols(&rows[li], selA, selB)
		if !ok {
			continue
		}
		tintCells(s, editorTop+i, textX+fromCol, textX+toCol, w, th)
	}
}

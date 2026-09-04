package composer

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/voice"
)

const (
	// holdThreshold separates a tap from a hold: a Space released before it
	// leaves the flip standing, a Space held past it flips back on release.
	holdThreshold = 300 * time.Millisecond
	// tapRepeatWindow is the fallback for terminals that do not report event
	// types, where a repeat looks exactly like a press and releases never
	// arrive: it is how long a press keeps swallowing further presses. It has
	// to outlast the terminal's first auto-repeat delay (375 ms by default on
	// macOS), and because the window slides with every repeat a hold of any
	// length still reads as one tap. 600 ms covers the three fastest of
	// macOS's six delay settings.
	tapRepeatWindow = 600 * time.Millisecond
	// holdRepeatWindow is the same fallback where releases do arrive. There
	// the release ends the press, so the window only matters when one is lost:
	// after two seconds of silence the next press is a new press rather than
	// a swallowed repeat.
	holdRepeatWindow = 2 * time.Second
	// meterGap is the blank field kept after the meter, so the row never
	// touches the next word when every block is lit.
	meterGap = 2
)

// meterBlocks is the level ramp drawn while the microphone listens, quietest
// first.
var meterBlocks = []rune{'▁', '▂', '▃', '▅', '▆', '▇'}

// SetVoice wires the microphone controller. The editor owns the session; the
// composer only drives the dialog mode and shows what it is doing.
func (c *ComposerPane) SetVoice(v VoiceController) {
	if c != nil {
		c.voice = v
	}
}

// VoiceState reports what the composer believes the microphone is doing.
func (c *ComposerPane) VoiceState() voice.State {
	if c == nil {
		return voice.StateIdle
	}
	return c.voiceState
}

// ToggleVoice is the Ctrl+G action: enter the dialog mode, or leave it keeping
// what was said. Entering closes the pickers, because the mode owns Space and
// the composer chrome.
func (c *ComposerPane) ToggleVoice() {
	if c == nil || c.voice == nil {
		return
	}
	if c.voiceState == voice.StateIdle {
		c.HideCompleters()
		c.voice.VoiceStart()
		return
	}
	c.voice.VoiceEnd()
}

// DiscardVoice leaves the mode and throws away everything not yet inserted.
// Local state is cleared at once and every event of the abandoned mode is
// ignored from here on, the way stale mention results are.
func (c *ComposerPane) DiscardVoice() {
	if c == nil || c.voice == nil {
		return
	}
	c.voice.VoiceDiscard()
	c.voiceMinGen = c.voiceGen + 1
	c.resetVoice()
}

// escapeVoice answers Esc while the mode is on: it takes back a pending send
// first, and only then leaves the mode. It reports whether Esc was used up.
func (c *ComposerPane) escapeVoice() bool {
	if c == nil || c.voiceState == voice.StateIdle {
		return false
	}
	if c.voiceSubmitPending {
		c.voiceSubmitPending = false
		c.applyHints()
		return true
	}
	c.DiscardVoice()
	return true
}

// handleVoiceKey answers the two keys the dialog mode owns, Space and Enter.
// It reports whether the event was used up. Space is a control key only where
// the composer would otherwise type it: no picker open, no modifiers.
func (c *ComposerPane) handleVoiceKey(ctx *components.EventContext, ev xui.KeyEvent) bool {
	if c.voiceState == voice.StateIdle || c.slash.Open || c.mention.Open || c.palette.Open {
		return false
	}
	if ev.Mods != 0 {
		return false
	}
	switch {
	case ev.Code == xui.KeyRune && ev.Rune == ' ':
		if ev.Press {
			c.pressSpace(ev.Repeat)
		} else {
			c.releaseSpace()
		}
	case ev.Press && ev.Code == xui.KeyEnter:
		c.submitVoice()
	default:
		return false
	}
	ctx.ConsumeAndRedraw()
	return true
}

// pressSpace flips the microphone at once, unless the press is auto-repeat.
// A terminal that reports event types says which it is, and that answer stands
// whatever the timings say. Where it does not, a press arriving while Space is
// down and inside the repeat window is read as a repeat. Either way the repeat
// only slides the window, so a key held down for a minute is still the single
// flip the user asked for.
func (c *ComposerPane) pressSpace(repeat bool) {
	now := c.clock()
	if repeat || (c.spaceDown && now.Sub(c.lastSpacePress) < c.repeatWindow()) {
		c.lastSpacePress = now
		return
	}
	c.spaceDown = true
	c.spacePressedAt = now
	c.lastSpacePress = now
	c.flipVoice()
}

// repeatWindow is how long a press keeps swallowing the next one where the
// terminal never says which is which. It is wide where releases end a press
// and narrow where the auto-repeat is all the composer ever hears.
func (c *ComposerPane) repeatWindow() time.Duration {
	if c.releasesSeen {
		return holdRepeatWindow
	}
	return tapRepeatWindow
}

// releaseSpace flips the microphone back when the key was held, which is what
// makes hold-to-pause and push-to-talk one rule. A release with no press
// behind it belongs to some earlier state and is ignored.
func (c *ComposerPane) releaseSpace() {
	if !c.spaceDown {
		return
	}
	c.spaceDown = false
	if c.clock().Sub(c.spacePressedAt) >= holdThreshold {
		c.flipVoice()
		return
	}
	c.applyHints()
}

// flipVoice pauses a listening microphone and resumes a paused one. The mirror
// moves with the call instead of waiting for the session's event, because a
// hold flips twice and the second flip must see the first.
func (c *ComposerPane) flipVoice() {
	if c.voice == nil {
		return
	}
	switch c.voiceState {
	case voice.StatePaused:
		c.voiceState = voice.StateListening
		c.voice.VoiceResume()
	case voice.StateListening:
		c.voiceState = voice.StatePaused
		c.voiceLevel = 0
		c.voice.VoicePause()
	default:
		// Idle never reaches here and Finishing is already leaving.
		return
	}
	c.applyHints()
}

// submitVoice is Enter in the mode: close the open segment and send once the
// queue is empty. It waits whenever the microphone is listening, because the
// words just spoken are not in the queue yet — the flush is what puts them
// there, and the session answers with a state event either way.
func (c *ComposerPane) submitVoice() {
	c.voice.VoiceFlush()
	if c.voicePending == 0 && c.voiceState != voice.StateListening {
		c.fireVoiceSubmit()
		return
	}
	c.voiceSubmitPending = true
	c.applyHints()
}

// fireVoiceSubmit sends the composer, unless there is nothing to send.
func (c *ComposerPane) fireVoiceSubmit() {
	c.voiceSubmitPending = false
	if c.Chat.OnSubmit != nil && strings.TrimSpace(c.Chat.Value) != "" {
		c.Chat.OnSubmit(c.Chat.Value)
	}
}

// ApplyVoiceState mirrors the session. Events from an abandoned mode are
// dropped; a pending send leaves as soon as the queue is empty.
func (c *ComposerPane) ApplyVoiceState(msg controller.VoiceStateMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	c.voiceGen = msg.Gen
	c.voiceState = msg.State
	c.voiceLevel = msg.Level
	c.voicePending = msg.Pending
	c.voiceStarting = msg.Starting
	if msg.State == voice.StateIdle {
		c.voiceLevel, c.voicePending = 0, 0
		c.voiceStarting, c.spaceDown = false, false
	}
	if c.voiceSubmitPending && c.voicePending == 0 {
		c.fireVoiceSubmit()
	}
	c.syncChatVoiceMode()
	c.applyHints()
}

// ApplyVoiceResult inserts one segment's transcript at the caret. The mode
// stays on: the next sentence is already being recorded.
func (c *ComposerPane) ApplyVoiceResult(msg controller.VoiceResultMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	if text := strings.TrimSpace(msg.Text); text != "" {
		c.insertAtCaret(text)
	}
}

// ApplyVoiceError takes back a pending send, so the user reads the toast the
// editor raised before anything is sent. The mode itself survives a segment
// that could not be transcribed.
func (c *ComposerPane) ApplyVoiceError(msg controller.VoiceErrorMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	c.voiceSubmitPending = false
	c.applyHints()
}

// resetVoice forgets the mode and gives the hint row and placeholder back.
func (c *ComposerPane) resetVoice() {
	c.voiceState = voice.StateIdle
	c.voiceLevel, c.voicePending = 0, 0
	c.voiceStarting, c.spaceDown, c.voiceSubmitPending = false, false, false
	c.syncChatVoiceMode()
	c.applyHints()
}

// syncChatVoiceMode tells the chat input whether the mode is on. Dispatch
// hands a key to the focused widget — the chat input while typing — before
// the pane ladder sees it, so the input has to defer Space and Enter itself
// or handleVoiceKey is never reached. Only the transitions in and out of
// StateIdle matter here: flipVoice stays inside the mode.
func (c *ComposerPane) syncChatVoiceMode() {
	c.Chat.VoiceMode = c.voiceState != voice.StateIdle
}

// VoiceHoldKeys reports whether key releases reach the app, which is what
// hold-to-pause and push-to-talk are built on. The editor's /voice status
// reads it here, because the composer is where the answer is learnt.
func (c *ComposerPane) VoiceHoldKeys() bool { return c != nil && c.releasesSeen }

// voiceHoldKeys is the same answer for the composer's own rows.
func (c *ComposerPane) voiceHoldKeys() bool {
	return c.releasesSeen
}

// clock reads the composer's time source; tests replace it so the tap and
// hold rules need no sleeps.
func (c *ComposerPane) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// insertAtCaret inserts the transcript as one replacement, so a single undo
// or one selection removes it. Spacing follows the neighbors: a word before
// the caret gets a space in front, a word after it gets one behind.
func (c *ComposerPane) insertAtCaret(text string) {
	at := min(max(c.Chat.Cursor, 0), len(c.Chat.Value))
	before, after := c.Chat.Value[:at], c.Chat.Value[at:]
	if r, _ := utf8.DecodeLastRuneInString(before); before != "" && !unicode.IsSpace(r) {
		text = " " + text
	}
	if r, _ := utf8.DecodeRuneInString(after); after != "" && !unicode.IsSpace(r) {
		text += " "
	}
	c.Chat.ReplaceRange(at, at, text)
}

// applyHints picks what the composer's right hint row and placeholder show:
// the microphone while the mode is on, otherwise the attachment or usage hint
// and the posture's placeholder.
func (c *ComposerPane) applyHints() {
	if spans := c.voiceHints(c.chatWidth); len(spans) > 0 {
		c.Chat.HintsRight = spans
	} else {
		c.Chat.HintsRight = c.hintsBase
	}
	c.Chat.Placeholder = c.placeholder()
}

// placeholder is the voice placeholder while the mode is on, the posture's
// one otherwise.
func (c *ComposerPane) placeholder() string {
	if text := c.voicePlaceholder(); text != "" {
		return text
	}
	if c.placeholderBase != "" {
		return c.placeholderBase
	}
	return askPlaceholder
}

// voicePlaceholder names the state in the words the user needs, or "" when
// the placeholder belongs to the posture.
func (c *ComposerPane) voicePlaceholder() string {
	switch {
	case c.voiceState == voice.StateIdle || c.voiceState == voice.StateFinishing:
		return ""
	case c.talking():
		return "Talking…"
	case c.voiceState == voice.StatePaused:
		return "Paused. Space to resume"
	default:
		return "Listening… speak, or type"
	}
}

// talking reports the held-key rows: the user is holding Space and the
// microphone is listening because of it.
func (c *ComposerPane) talking() bool {
	return c.spaceDown && c.voiceHoldKeys() && c.voiceState == voice.StateListening
}

// voiceHints builds the right hint row for the mode, or nothing when it is
// off. The key hint is a span of its own and goes first when the row does not
// fit, because paintHintsRow right-aligns without truncating.
func (c *ComposerPane) voiceHints(width int) []components.Span {
	spans, hint := c.voiceRow()
	if len(spans) == 0 || hint == "" {
		return spans
	}
	full := append(spans, components.Span{Text: hint, Style: c.theme.Muted})
	if width <= 0 || components.MeasureSpans(full, c.chatMethod) <= width-2 {
		return full
	}
	return spans
}

// voiceRow is the hint row split into the part that always shows and the key
// hint that may be dropped. Precedence runs from the states that are leaving
// the mode to the ones that stay in it.
func (c *ComposerPane) voiceRow() ([]components.Span, string) {
	hold := c.voiceHoldKeys()
	switch {
	case c.voiceState == voice.StateIdle:
		return nil, ""
	case c.voiceSubmitPending:
		return []components.Span{{Text: "⋯ finishing… then send", Style: c.theme.Muted}}, "  Esc cancel"
	case c.voiceState == voice.StateFinishing:
		return []components.Span{{Text: "⋯ finishing…", Style: c.theme.Muted}}, ""
	case c.voiceStarting:
		return []components.Span{
			{Text: "● ", Style: c.theme.Destructive},
			{Text: "starting…", Style: c.theme.Muted},
		}, ""
	case c.talking():
		return c.meterSpans("talking"), "  release to pause"
	case c.voiceState == voice.StatePaused && c.spaceDown && hold:
		return c.pausedSpans(""), "  release to resume"
	case c.voiceState == voice.StatePaused && hold:
		return c.pausedSpans(c.queueText()), "  Space resume · hold to talk · ^G done"
	case c.voiceState == voice.StatePaused:
		return c.pausedSpans(c.queueText()), "  Space resume · ^G done"
	default:
		return c.meterSpans(c.queueText()), "  Space pause · ^G done"
	}
}

// meterSpans is the listening row: the dot, the level, and one word about
// what the microphone is doing.
func (c *ComposerPane) meterSpans(note string) []components.Span {
	spans := []components.Span{
		{Text: "● ", Style: c.theme.Destructive},
		{Text: meterBar(c.voiceLevel), Style: c.theme.Muted},
	}
	if note != "" {
		spans = append(spans, components.Span{Text: note, Style: c.theme.Muted})
	}
	return spans
}

// pausedSpans is the paused row.
func (c *ComposerPane) pausedSpans(note string) []components.Span {
	spans := []components.Span{
		{Text: "‖ ", Style: c.theme.Warning},
		{Text: "paused", Style: c.theme.Muted},
	}
	if note != "" {
		spans = append(spans, components.Span{Text: "  " + note, Style: c.theme.Muted})
	}
	return spans
}

// queueText names the segments still waiting for the transcriber, including
// the one in flight, and says nothing when there are none.
func (c *ComposerPane) queueText() string {
	if c.voicePending <= 0 {
		return ""
	}
	return fmt.Sprintf("⋯%d", c.voicePending)
}

// meterBar draws a 0..1 level as a fixed-width ramp followed by a blank gap,
// so the hint row does not jitter as the voice rises and falls.
func meterBar(level float64) string {
	switch {
	case level < 0:
		level = 0
	case level > 1:
		level = 1
	}
	filled := min(int(level*float64(len(meterBlocks))+0.5), len(meterBlocks))
	var b strings.Builder
	b.WriteString(string(meterBlocks[:filled]))
	b.WriteString(strings.Repeat(" ", len(meterBlocks)-filled+meterGap))
	return b.String()
}

package composer

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// meterBlocks is the level ramp drawn while recording, quietest first.
var meterBlocks = []rune{'▁', '▂', '▃', '▅', '▆', '▇'}

// SetVoice wires the microphone controller. The editor owns the session; the
// composer only toggles it and shows what it is doing.
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

// ToggleVoice is the Ctrl+G action: start a recording, stop the running one,
// or let the session say that a transcription is still in flight. Starting
// closes the pickers, because the recording owns the composer chrome.
func (c *ComposerPane) ToggleVoice() {
	if c == nil || c.voice == nil {
		return
	}
	if c.voiceState == voice.StateIdle {
		c.voiceEmptyBefore = strings.TrimSpace(c.Chat.Value) == ""
		c.HideCompleters()
	}
	c.voice.ToggleVoice()
}

// StopVoice ends the recording and starts transcription. Enter uses it: the
// prompt in the composer is deliberately not sent.
func (c *ComposerPane) StopVoice() {
	if c != nil && c.voice != nil {
		c.voice.StopVoice()
	}
}

// CancelVoice throws the recording away without a word, which is what Esc
// means. Local state is cleared at once and every event of the abandoned
// recording is ignored from here on.
func (c *ComposerPane) CancelVoice() {
	if c == nil || c.voice == nil {
		return
	}
	c.voice.CancelVoice()
	c.voiceMinGen = c.voiceGen + 1
	c.voiceEmptyBefore = false
	c.clearVoiceMeter()
}

// ApplyVoiceState moves the meter. Events from an abandoned recording are
// dropped, the way stale mention results are.
func (c *ComposerPane) ApplyVoiceState(msg controller.VoiceStateMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	c.voiceGen = msg.Gen
	c.voiceState = msg.State
	c.voiceElapsed = msg.Elapsed
	c.voiceLevel = msg.Level
	if msg.State == voice.StateIdle {
		c.voiceElapsed, c.voiceLevel = 0, 0
	}
	c.applyHints()
}

// ApplyVoiceResult inserts a finished transcript at the caret, and sends it
// only when voice.auto_send is on and the composer was empty when recording
// began.
func (c *ComposerPane) ApplyVoiceResult(msg controller.VoiceResultMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	text := strings.TrimSpace(msg.Text)
	c.voiceState = voice.StateIdle
	c.clearVoiceMeter()
	if text == "" {
		c.voiceEmptyBefore = false
		return
	}
	c.insertAtCaret(text)
	autoSend := c.voiceEmptyBefore && c.voice != nil && c.voice.VoiceAutoSend()
	c.voiceEmptyBefore = false
	if autoSend && c.Chat.OnSubmit != nil {
		c.Chat.OnSubmit(c.Chat.Value)
	}
}

// ApplyVoiceError clears the meter; the error text itself is a toast the
// editor raises, so the composer only has to stop pretending to record.
func (c *ComposerPane) ApplyVoiceError(msg controller.VoiceErrorMsg) {
	if c == nil || msg.Gen < c.voiceMinGen {
		return
	}
	c.voiceState = voice.StateIdle
	c.voiceEmptyBefore = false
	c.clearVoiceMeter()
}

// clearVoiceMeter drops the meter and restores whatever hint it covered.
func (c *ComposerPane) clearVoiceMeter() {
	c.voiceState = voice.StateIdle
	c.voiceElapsed, c.voiceLevel = 0, 0
	c.applyHints()
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

// applyHints picks what the composer's right hint row shows: the voice meter
// while the microphone is busy, otherwise the attachment or usage hint that
// was there before.
func (c *ComposerPane) applyHints() {
	if spans := c.voiceHints(); len(spans) > 0 {
		c.Chat.HintsRight = spans
		return
	}
	c.Chat.HintsRight = c.hintsBase
}

// voiceHints renders the meter, or nothing when the microphone is idle.
func (c *ComposerPane) voiceHints() []components.Span {
	switch c.voiceState {
	case voice.StateRecording:
		return []components.Span{
			{Text: "● ", Style: c.theme.Destructive},
			{Text: meterBar(c.voiceLevel) + " " + formatMeterTime(c.voiceElapsed), Style: c.theme.Muted},
		}
	case voice.StateTranscribing:
		return []components.Span{{Text: "⋯ transcribing…", Style: c.theme.Muted}}
	case voice.StateIdle:
		return nil
	default:
		return nil
	}
}

// meterBar draws a 0..1 level as a fixed-width ramp, so the hint row does not
// jitter as the voice rises and falls.
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
	b.WriteString(strings.Repeat(" ", len(meterBlocks)-filled))
	return b.String()
}

// formatMeterTime renders the elapsed recording time as mm:ss.
func formatMeterTime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

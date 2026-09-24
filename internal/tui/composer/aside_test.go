package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/voice"
)

func TestAsideModeSubmitsExactlyOneQuestionWithoutPublishingPrompt(t *testing.T) {
	c, bus := wiredPane(t)
	var anchor, question string
	c.SetAsideSubmit(func(a, q string) bool { anchor, question = a, q; return true })
	c.EnterAside("answer-id")
	require.Equal(t, "⏵⏵ btw @answer-id", c.Chat.AgentLabel.Text)
	require.True(t, c.Chat.AgentLabel.Style.Equal(c.theme.Aside))
	require.Equal(t, asidePlaceholder, c.Chat.Placeholder)
	c.Chat.Value = "why?"
	c.Chat.Cursor = len(c.Chat.Value)
	c.Chat.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	require.Equal(t, "answer-id", anchor)
	require.Equal(t, "why?", question)
	require.Nil(t, bus.published, "aside must not enter the regular prompt pipeline")
	require.Empty(t, c.Chat.Value)
	require.Equal(t, "⏵⏵ useplan", c.Chat.AgentLabel.Text)
}

func TestAsideRefusalKeepsDraftAndModeForRetry(t *testing.T) {
	c, bus := wiredPane(t)
	c.SetAsideSubmit(func(string, string) bool { return false })
	c.EnterAside("answer-id")
	c.Chat.Value = "try again?"
	c.Chat.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	require.Equal(t, "try again?", c.Chat.Value)
	require.Equal(t, "⏵⏵ btw @answer-id", c.Chat.AgentLabel.Text)
	require.Nil(t, bus.published)
}

func TestAsideEscapeKeepsDraftAndClearsAnchor(t *testing.T) {
	c, bus := wiredPane(t)
	c.SetAsideSubmit(func(string, string) bool { t.Fatal("cancelled aside submitted"); return false })
	c.EnterAside("old-answer")
	c.Chat.Value = "draft"
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEscape, Press: true})
	require.True(t, ctx.Consume)
	require.Equal(t, "draft", c.Chat.Value)
	require.Equal(t, "⏵⏵ useplan", c.Chat.AgentLabel.Text)
	require.Nil(t, bus.published)
}

func TestAsideChordAndBareSlashToggleMode(t *testing.T) {
	c, _ := wiredPane(t)
	c.SetAsideSubmit(func(string, string) bool { return true })
	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 't', Mods: xui.ModCtrl, Press: true})
	require.Equal(t, "⏵⏵ btw", c.Chat.AgentLabel.Text)
	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 't', Mods: xui.ModCtrl, Press: true})
	require.Equal(t, "⏵⏵ useplan", c.Chat.AgentLabel.Text)
	require.Equal(t, "Ctrl+T", keys.Label(keys.CmdAside))
	require.Contains(t, c.Chat.LeadTooltip(), keys.Label(keys.CmdAside))
}

func TestAsideCannotOverlapVoiceOrShellPrefix(t *testing.T) {
	c, _ := wiredPane(t)
	c.SetAsideSubmit(func(string, string) bool { return true })
	c.Chat.Value = "!ls"
	c.EnterAside("")
	require.NotContains(t, c.Chat.AgentLabel.Text, "btw")
	c.Chat.Value = ""
	c.voiceState = voice.StateListening
	c.EnterAside("")
	require.NotContains(t, c.Chat.AgentLabel.Text, "btw")
	c.voiceState = voice.StateIdle
	c.SetBashBorderActive(true)
	require.False(t, c.EnterAside(""))
	require.False(t, c.asideActive)
}

func TestAsideVoiceStartLeavesModeWithoutDroppingDraft(t *testing.T) {
	c, _ := wiredPane(t)
	mic := &fakeVoice{}
	c.SetVoice(mic)
	c.SetAsideSubmit(func(string, string) bool { return true })
	require.True(t, c.EnterAside("answer-id"))
	c.Chat.Value = "draft"
	c.ToggleVoice()
	require.Equal(t, 1, mic.starts)
	require.Equal(t, "draft", c.Chat.Value)
	require.Equal(t, "⏵⏵ useplan", c.Chat.AgentLabel.Text)
}

func TestAsideLeavesModeBeforeAddingMediaSkillOrShell(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(*ComposerPane)
	}{
		{"image", func(c *ComposerPane) { c.AttachMedia(llm.Media{MediaType: "image/png"}) }},
		{"skill", func(c *ComposerPane) { c.AddPendingSkill("audit") }},
		{"shell", func(c *ComposerPane) { c.SetBashBorderActive(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := wiredPane(t)
			c.SetAsideSubmit(func(string, string) bool { return true })
			require.True(t, c.EnterAside("answer-id"))
			c.Chat.Value = "draft"
			tc.act(c)
			require.False(t, c.asideActive)
			require.NotContains(t, c.Chat.AgentLabel.Text, "btw")
			require.Equal(t, "draft", c.Chat.Value)
		})
	}
}

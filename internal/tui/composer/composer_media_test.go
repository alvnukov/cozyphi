package composer

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/clipboard"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestComposerAttachMediaSubmittedWithText(t *testing.T) {
	c := newTestPane()
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.AttachMedia(llm.Media{MediaType: "image/png", Data: "AAECAw=="})
	require.Len(t, c.AttachedMedia(), 1)

	c.Chat.OnSubmit("look at this")

	require.Equal(t, controller.SubmitMsg{
		Text:  "look at this",
		Media: []llm.Media{{MediaType: "image/png", Data: "AAECAw=="}},
	}, bus.published)
	require.Empty(t, c.AttachedMedia())
}

func TestComposerClearAttachedMedia(t *testing.T) {
	c := newTestPane()
	c.AttachMedia(llm.Media{MediaType: "image/png", Data: "AAECAw=="})
	require.Len(t, c.AttachedMedia(), 1)

	c.ClearAttachedMedia()
	require.Empty(t, c.AttachedMedia())
}

func TestComposerCtrlVPasteImage(t *testing.T) {
	c := newTestPane()
	c.readClipboard = func() (clipboard.Image, bool, error) {
		return clipboard.Image{Data: []byte{0x89, 'P', 'N', 'G'}, MediaType: "image/png"}, true, nil
	}
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Handle(&components.EventContext{}, xui.KeyEvent{
		Press: true,
		Code:  xui.KeyRune,
		Rune:  'v',
		Mods:  xui.ModCtrl,
	})

	require.Len(t, c.AttachedMedia(), 1)
	require.Equal(t, "image/png", c.AttachedMedia()[0].MediaType)
}

func TestComposerCtrlVPasteNoImageFallsThrough(t *testing.T) {
	c := newTestPane()
	c.readClipboard = func() (clipboard.Image, bool, error) {
		return clipboard.Image{}, false, nil
	}
	bus := &fakeBus{}
	c.Wire(nil, nil, nil, "", bus, &fakeFocus{})

	c.Handle(&components.EventContext{}, xui.KeyEvent{
		Press: true,
		Code:  xui.KeyRune,
		Rune:  'v',
		Mods:  xui.ModCtrl,
	})

	require.Empty(t, c.AttachedMedia())
}

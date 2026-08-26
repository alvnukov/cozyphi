package composer

import (
	"testing"

	"github.com/stretchr/testify/require"

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

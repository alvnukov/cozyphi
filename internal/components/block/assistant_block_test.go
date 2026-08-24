package block

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/session"
)

// cellCheck spot-checks the style painted at one buffer cell.
type cellCheck struct {
	x, y  int
	style xui.Style
}

func TestAssistantBlockDraw(t *testing.T) {
	th := components.DefaultTheme()
	pathSt := th.Foreground // paths keep the prose color; underline is the affordance
	pathSt.Underline = true

	tests := []struct {
		name           string
		text           string
		state          session.State
		width          int // Max.Width; <= 0 exercises the 40 default
		wantWidth      int
		wantMinHeight  int
		wantContains   []string
		wantNotContain []string
		wantCells      []cellCheck
	}{
		{
			name:         "plain text",
			text:         "hello world",
			width:        60,
			wantWidth:    60,
			wantContains: []string{"hello world"},
		},
		{
			name:         "empty width falls back to 40",
			text:         "hi",
			width:        0,
			wantWidth:    40,
			wantContains: []string{"hi"},
		},
		{
			name:          "hard newlines produce rows",
			text:          "line one\nline two",
			width:         60,
			wantWidth:     60,
			wantMinHeight: 2,
			wantContains:  []string{"line one", "line two"},
		},
		{
			name:          "long text soft-wraps",
			text:          strings.Repeat("word ", 20),
			width:         12,
			wantWidth:     12,
			wantMinHeight: 3,
			wantContains:  []string{"word word"},
		},
		{
			name:           "inline code strips backticks and highlights",
			text:           "run `go test` now",
			width:          60,
			wantWidth:      60,
			wantContains:   []string{"run ", "go test", " now"},
			wantNotContain: []string{"`"},
			wantCells: []cellCheck{
				{x: messageIndent, y: 0, style: th.Foreground},              // "run "
				{x: messageIndent + 4, y: 0, style: th.Markdown.InlineCode}, // code token
			},
		},
		{
			name:         "paths highlighted",
			text:         "see internal/components/block.go",
			width:        60,
			wantWidth:    60,
			wantContains: []string{"internal/components/block.go"},
			wantCells: []cellCheck{
				{x: messageIndent + 4, y: 0, style: pathSt}, // path token start
			},
		},
		{
			name:          "cancelled appends muted label",
			text:          "partial",
			state:         session.StateCancelled,
			width:         60,
			wantWidth:     60,
			wantMinHeight: 2,
			wantContains:  []string{"partial", "cancelled"},
			wantCells: []cellCheck{
				{x: messageIndent, y: 1, style: th.Muted}, // "cancelled" row
			},
		},
		{
			name:           "cancelled with empty text renders no label",
			state:          session.StateCancelled,
			width:          60,
			wantWidth:      60,
			wantNotContain: []string{"cancelled"},
		},
		{
			name:           "complete state renders no label",
			text:           "done",
			state:          session.StateComplete,
			width:          60,
			wantWidth:      60,
			wantContains:   []string{"done"},
			wantNotContain: []string{"cancelled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assistantBlock := &AssistantBlock{
				Text:  tt.text,
				State: tt.state,
				Theme: th,
			}
			surface := assistantBlock.Draw(components.DrawContext{
				Max:    components.Size{Width: tt.width},
				Method: xui.WidthUnicode,
			})

			// The surface must carry the block as its widget identity.
			assert.Same(t, assistantBlock, surface.Widget)

			assert.Equal(t, tt.wantWidth, surface.Size.Width, "surface width")
			assert.GreaterOrEqual(t, surface.Size.Height, max(1, tt.wantMinHeight), "surface height")

			txt := components.SurfaceText(surface)
			for _, want := range tt.wantContains {
				assert.Contains(t, txt, want)
			}
			for _, notWant := range tt.wantNotContain {
				assert.NotContains(t, txt, notWant)
			}

			for _, cc := range tt.wantCells {
				if cc.y >= surface.Size.Height || cc.x >= surface.Size.Width {
					t.Fatalf("cell (%d,%d) outside surface %dx%d", cc.x, cc.y, surface.Size.Width, surface.Size.Height)
				}
				got := surface.Buffer[cc.y*surface.Size.Width+cc.x]
				assert.True(
					t,
					cc.style.Equal(got.Style),
					"style at (%d,%d): want %+v, got %+v (char %q)",
					cc.x,
					cc.y,
					cc.style,
					got.Style,
					got.Char,
				)
			}
		})
	}
}

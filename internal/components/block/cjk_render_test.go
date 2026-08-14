package block_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/block"
)

const cjkHowTo = `【怎么做】
1. 翻出几篇你满意的笔记，找出共同点：标题怎么起、开头几句话说什么、分几段、写多少字
2. 把规律一条条写清楚。越具体越好：别写"写得好一点"，要写"每段1-2行，全篇不超过800字"
3. 把每次会变的部分用{{占位符}}代替，比如{{主题}}
4. 让AI照着写一篇试试，哪里不对就改说明书，改到满意为止`

func TestCJKSampleNoBlankGapsAfterClearRender(t *testing.T) {
	th := components.DefaultTheme()
	a := &block.AssistantBlock{Text: cjkHowTo, Theme: th}
	const w = 80
	s := a.Draw(components.DrawContext{Max: components.Size{Width: w, Height: 20}, Method: xui.WidthUnicode})

	screen := xui.NewScreen(w, s.Size.Height+2)
	win := xui.NewWindow(screen)
	win.Clear()
	s.Render(win)

	txt := components.SurfaceText(s)
	for _, want := range []string{"翻出几篇你满意的", "把每次", "笔记"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("surface missing %q", want)
		}
	}

	var b strings.Builder
	_, h := screen.Size()
	for y := range h {
		for x := 0; x < w; {
			c := screen.GetCell(x, y)
			step := int(c.Width)
			step = max(step, 1)
			if !c.Trail && c.Char != "" && c.Char != " " {
				_, _ = b.WriteString(c.Char)
			}
			x += step
		}
		_ = b.WriteByte('\n')
	}
	got := b.String()
	for _, want := range []string{"翻出几篇你满意的", "把每次"} {
		if !strings.Contains(got, want) {
			t.Fatalf("screen missing %q\n%s", want, got)
		}
	}

	screen.MarkRefresh()
	for _, d := range screen.Diff() {
		if d.Cell.Trail {
			t.Fatalf("Diff emitted Trail at (%d,%d)", d.X, d.Y)
		}
	}
}

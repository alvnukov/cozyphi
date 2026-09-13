// Package shellpane renders the session's shell tasks from immutable snapshots.
package shellpane

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// Pane owns presentation and selection; process operations remain with the controller.
type Pane struct {
	theme      components.Theme
	tasks      []shelltask.Snapshot
	sessionID  string
	background func(string) error
	stop       func(string) error
	close      func()
	visible    bool
	outputID   string
	follow     bool
	notice     string
	cursor     browse.Cursor
	motions    browse.Motions
	confirm    browse.Confirm
	output     browse.Scroller
}

// New constructs a hidden pane with callbacks for its three user actions.
func New(theme components.Theme, background, stop func(string) error, closePane func()) *Pane {
	return &Pane{theme: theme, background: background, stop: stop, close: closePane}
}

// SetTasks applies a complete snapshot, preserving the selected task's identity.
func (p *Pane) SetTasks(tasks []shelltask.Snapshot) {
	selected, _ := p.selected()
	p.tasks = slices.Clone(tasks)
	p.cursor.SetRows(len(p.tasks), nil)
	if selected.ID != "" {
		for i, task := range p.tasks {
			if task.ID == selected.ID && i != p.cursor.Selected() {
				p.cursor.Select(i)
				break
			}
		}
	}
}

// SetSessionID identifies the current conversation without hiding other tasks.
func (p *Pane) SetSessionID(id string) { p.sessionID = id }

// Show opens the task list; its current selection survives a reopen.
func (p *Pane) Show() {
	p.visible = true
	p.outputID = ""
	p.notice = ""
	p.confirm.Disarm()
	p.motions.Reset()
}

// Hide closes the pane and returns focus once.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	p.confirm.Disarm()
	if p.close != nil {
		p.close()
	}
}

// Visible reports whether the pane owns the screen.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// SetTheme updates presentation without disturbing selection or output.
func (p *Pane) SetTheme(theme components.Theme) { p.theme = theme }

// Handle is inert because the session shell dispatches through HandleEvent.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent consumes input while visible, including input in the output view.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if !p.Visible() {
		return false
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Press {
			p.handleKey(e)
			ctx.ConsumeAndRedraw()
		}
		return true
	case xui.MouseEvent:
		if p.outputID == "" {
			p.cursor.Wheel(e)
		} else if motion, ok := browse.Wheel(e); ok {
			p.follow = false
			p.output.Apply(motion)
		}
		ctx.ConsumeAndRedraw()
		return true
	default:
		return false
	}
}

func (p *Pane) handleKey(e xui.KeyEvent) {
	p.notice = ""
	if p.outputID != "" {
		if e.Code == xui.KeyEscape || e.Code == xui.KeyEnter ||
			(e.Code == xui.KeyRune && e.Mods == 0 && e.HotkeyRune() == 'q') {
			p.outputID = ""
			p.motions.Reset()
			return
		}
		if motion, ok := p.motions.Key(e); ok {
			p.follow = motion.Op == browse.OpBottom
			p.output.Apply(motion)
		}
		return
	}
	if p.confirm.Key(e) {
		return
	}
	if motion, ok := p.motions.Key(e); ok {
		p.cursor.Apply(motion)
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyEnter:
		if task, ok := p.selected(); ok {
			p.outputID, p.follow = task.ID, true
			p.motions.Reset()
		}
	case xui.KeyRune:
		if e.Mods != 0 {
			return
		}
		switch e.HotkeyRune() {
		case 'q':
			p.Hide()
		case 'b':
			p.backgroundSelected()
		case 's':
			p.stopSelected()
		}
	}
}

func (p *Pane) selected() (shelltask.Snapshot, bool) {
	if len(p.tasks) == 0 {
		return shelltask.Snapshot{}, false
	}
	return p.tasks[p.cursor.Selected()], true
}

func (p *Pane) backgroundSelected() {
	task, ok := p.selected()
	if !ok {
		return
	}
	if task.State != shelltask.Running || task.Background {
		p.notice = "select a running foreground command"
		return
	}
	if p.background != nil {
		if err := p.background(task.ID); err != nil {
			p.notice = "cannot background command: " + err.Error()
		} else {
			p.notice = "background requested"
		}
	}
}

func (p *Pane) stopSelected() {
	task, ok := p.selected()
	if !ok {
		return
	}
	if task.State != shelltask.Running {
		p.notice = "this command already ended"
		return
	}
	// Capture the identity now: a completion can reorder the list before y arrives.
	p.confirm.Arm(fmt.Sprintf("stop command %q?", task.Command), func() {
		if p.stop != nil {
			if err := p.stop(task.ID); err != nil {
				p.notice = "cannot stop command: " + err.Error()
			} else {
				p.notice = "stop requested"
			}
		}
	})
}

// Draw renders snapshots only; the existing scheduler owns elapsed-time wakes.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	width, height := max(ctx.Max.Width, 1), max(ctx.Max.Height, 1)
	surface := components.NewSurface(width, height, p)
	for y := range height {
		for x := range width {
			surface.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: p.theme.Foreground})
		}
	}
	put := func(y int, value string, style xui.Style) {
		if y >= 0 && y < height {
			surface.Print(
				1,
				y,
				layout.TruncateToWidth(
					strings.ReplaceAll(components.PlainText(value), "\n", " "),
					max(width-2, 0),
					ctx.Method,
				),
				style,
				ctx.Method,
			)
		}
	}
	if p.outputID != "" {
		p.drawOutput(height, put)
		return surface
	}
	put(0, "Shell tasks · /tasks · /bashes", p.theme.Warning)
	put(1, "state         id           command", p.theme.Muted)
	p.cursor.SetViewport(max(height-3, 0))
	if len(p.tasks) == 0 {
		put(2, "no shell tasks this session", p.theme.Muted)
	}
	for i := p.cursor.Scroll(); i < len(p.tasks) && i-p.cursor.Scroll() < height-3; i++ {
		task := p.tasks[i]
		state := stateLabel(task)
		elapsed := task.Finished.Sub(task.Started)
		if task.State == shelltask.Running {
			elapsed = time.Since(task.Started)
			ctx.WakeIn(time.Second)
		}
		label := fmt.Sprintf("%-13s %-12s %s", state, task.ID, task.Command)
		if !task.Started.IsZero() {
			label += " · " + components.FormatDuration(max(elapsed, 0))
		}
		if task.ParentSessionID != p.sessionID {
			label = "[other session] " + label
		}
		style := p.theme.Foreground
		if task.State == shelltask.Failed {
			style = p.theme.Destructive
		}
		if i == p.cursor.Selected() {
			style.Reverse = true
		}
		put(2+i-p.cursor.Scroll(), label, style)
	}
	hint := keys.Hints(keys.ScopeShellTasks)
	if p.confirm.Armed() {
		hint = p.confirm.Label() + " (y/n)"
	} else if p.notice != "" {
		hint = p.notice
	}
	put(height-1, hint, p.theme.Muted)
	return surface
}

func stateLabel(task shelltask.Snapshot) string {
	if task.State == shelltask.Running {
		if task.Background {
			return "background"
		}
		return "foreground"
	}
	if task.State == shelltask.Failed {
		return fmt.Sprintf("failed (%d)", task.ExitCode)
	}
	return string(task.State)
}

func (p *Pane) drawOutput(height int, put func(int, string, xui.Style)) {
	var task shelltask.Snapshot
	found := false
	for _, candidate := range p.tasks {
		if candidate.ID == p.outputID {
			task, found = candidate, true
			break
		}
	}
	if !found {
		put(0, "Shell output · task no longer retained", p.theme.Warning)
		put(height-1, keys.Hints(keys.ScopeShellOutput), p.theme.Muted)
		return
	}
	put(0, task.ID+" · "+stateLabel(task)+" · "+task.Command, p.theme.Warning)
	label := task.OutputFile
	if !task.Deadline.IsZero() {
		label = "expires " + task.Deadline.Format("15:04:05") + " · " + label
	}
	if task.ParentSessionID != p.sessionID {
		label = "other session · " + label
	}
	if task.Truncated {
		label = "showing bounded tail · " + label
	}
	put(1, label, p.theme.Muted)
	lines := strings.Split(components.PlainText(task.Output), "\n")
	p.output.SetExtent(len(lines), max(height-3, 0))
	if p.follow {
		p.output.Apply(browse.Motion{Op: browse.OpBottom})
	}
	for i := p.output.Offset(); i < len(lines) && i-p.output.Offset() < height-3; i++ {
		put(2+i-p.output.Offset(), lines[i], p.theme.Foreground)
	}
	put(height-1, keys.Hints(keys.ScopeShellOutput), p.theme.Muted)
}

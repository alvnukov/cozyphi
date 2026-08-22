package tui

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/debuglog"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// HookCommands owns slash commands registered from KindCommand hooks.
// Mirrors SessionActions: UI side effects live here, not on Editor.
type HookCommands struct {
	e       *Editor
	gen     atomic.Uint64
	running atomic.Bool
}

// Sync replaces hook-sourced slash commands from the current hooks.Manager.
func (h *HookCommands) Sync() {
	if h == nil || h.e == nil || h.e.commands == nil {
		return
	}
	e := h.e
	h.gen.Add(1)
	e.commands.clearHookCommands()
	if e.ctrl != nil {
		for _, entry := range e.ctrl.Hooks().CommandEntries() {
			name := entry.Hook.Name()
			if !e.commands.registerHook(h.slashCommand(name)) {
				debuglog.Logf("hooks: command %q skipped (name already registered)", name)
			}
		}
	}
	e.composer.SetPaletteCommands(e.commands.BuildPalette(e.commandContext()))
}

func (h *HookCommands) slashCommand(name string) Command {
	return Command{
		Name:        name,
		Description: "hook command",
		Slash:       true,
		Insert:      "/" + name,
		Run: func(ctx CommandContext) error {
			if h.running.Load() {
				ctx.toast("A hook command is already running", toast.ToastWarning, 3*time.Second)
				return nil
			}
			args := append([]string(nil), ctx.Args...)
			go h.run(name, args)
			return nil
		},
	}
}

func (h *HookCommands) run(name string, args []string) {
	e := h.e
	if !h.running.CompareAndSwap(false, true) {
		e.Publish(controller.HookCommandResultMsg{
			Gen: h.gen.Load(),
			Err: "A hook command is already running",
		})
		return
	}
	defer h.running.Store(false)

	gen := h.gen.Load()
	if e.ctrl == nil {
		e.Publish(controller.HookCommandResultMsg{Gen: gen, Err: "hooks are not loaded"})
		return
	}
	mgr := e.ctrl.Hooks()
	if mgr == nil {
		e.Publish(controller.HookCommandResultMsg{Gen: gen, Err: "hooks are not loaded"})
		return
	}
	res, err := mgr.RunCommand(context.Background(), name, hooks.CommandEvent{
		SessionID: e.ctrl.SessionID(),
		Cwd:       e.cwd,
		Args:      args,
	})
	if gen != h.gen.Load() {
		return
	}
	if err != nil {
		e.Publish(controller.HookCommandResultMsg{Gen: gen, Err: err.Error()})
		return
	}
	e.Publish(controller.HookCommandResultMsg{
		Gen:       gen,
		Submit:    res.Submit,
		Toast:     res.Toast,
		Status:    res.Status,
		StatusSet: res.StatusSet,
		List:      res.List,
	})
}

// Apply delivers a finished hook command onto the UI goroutine.
func (h *HookCommands) Apply(msg controller.HookCommandResultMsg) {
	if h == nil || h.e == nil || msg.Gen != h.gen.Load() {
		return
	}
	e := h.e
	if msg.Err != "" {
		e.toast.Show(msg.Err, toast.ToastError, 3*time.Second)
		return
	}
	h.applyIntents(msg)
}

// applyIntents applies status / toast / list / submit.
func (h *HookCommands) applyIntents(msg controller.HookCommandResultMsg) {
	e := h.e
	if msg.StatusSet {
		e.footer.SetHookStatus(msg.Status)
	}
	if msg.Toast != "" {
		e.toast.Show(msg.Toast, toast.ToastSuccess, 3*time.Second)
	}
	if msg.List != nil && len(msg.List.Items) > 0 {
		h.pushList(*msg.List)
		return
	}
	if msg.Submit != "" {
		if e.submitter.IsBusy() {
			e.toast.Show("Cannot submit hook command while a reply is running", toast.ToastWarning, 3*time.Second)
			return
		}
		e.submitter.Submit(msg.Submit)
	}
}

func (h *HookCommands) pushList(list hooks.CommandList) {
	e := h.e
	title := list.Title
	if title == "" {
		title = "Hook"
	}
	cmds := make([]palette.PaletteCommand, 0, len(list.Items))
	for _, item := range list.Items {
		label := item.Label
		if label == "" {
			label = item.Submit
		}
		if label == "" {
			continue
		}
		cmds = append(cmds, palette.PaletteCommand{
			Verb:     label,
			Keywords: keywordsForDetail(item.Detail),
			Run: func() {
				if item.Submit == "" {
					return
				}
				if e.submitter.IsBusy() {
					e.toast.Show("Cannot submit while a reply is running", toast.ToastWarning, 3*time.Second)
					return
				}
				e.submitter.Submit(item.Submit)
			},
		})
	}
	if len(cmds) == 0 {
		e.toast.Show("Hook list had no usable items", toast.ToastWarning, 3*time.Second)
		return
	}
	e.composer.PushPalette(title, cmds)
}

func keywordsForDetail(detail string) []string {
	if detail == "" {
		return nil
	}
	return []string{detail}
}

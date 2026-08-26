package commands

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type hookComposer interface {
	SetPaletteCommands([]palette.PaletteCommand)
	PushPalette(title string, cmds []palette.PaletteCommand)
}

type hookFooter interface {
	SetHookStatus(status string)
}

type hookSubmitter interface {
	CanSubmit() bool
	Submit(text string, media ...llm.Media)
}

// HookCommands owns slash commands registered from KindCommand hooks.
type HookCommands struct {
	Registry  *CommandRegistry
	Ctrl      *controller.Controller
	CWD       string
	Composer  hookComposer
	Footer    hookFooter
	Submitter hookSubmitter
	Toast     toast.Toast
	Publish   func(controller.Msg)
	Host      Host

	gen     atomic.Uint64
	running atomic.Bool
}

// NewHookCommands builds hook command handlers with every collaborator set at
// construction — no two-phase field assignment after the fact.
func NewHookCommands(
	registry *CommandRegistry,
	ctrl *controller.Controller,
	cwd string,
	composer hookComposer,
	footer hookFooter,
	submitter hookSubmitter,
	toast toast.Toast,
	publish func(controller.Msg),
	host Host,
) *HookCommands {
	return &HookCommands{
		Registry:  registry,
		Ctrl:      ctrl,
		CWD:       cwd,
		Composer:  composer,
		Footer:    footer,
		Submitter: submitter,
		Toast:     toast,
		Publish:   publish,
		Host:      host,
	}
}

// Sync replaces hook-sourced slash commands from the current hooks.Manager.
func (h *HookCommands) Sync() {
	if h == nil || h.Registry == nil {
		return
	}
	h.gen.Add(1)
	h.Registry.clearHookCommands()
	if h.Ctrl != nil {
		for _, entry := range h.Ctrl.Hooks().CommandEntries() {
			name := entry.Hook.Name()
			if !h.Registry.registerHook(h.slashCommand(name)) {
				debuglog.Logf("hooks: command %q skipped (name already registered)", name)
			}
		}
	}
	h.Composer.SetPaletteCommands(h.Registry.BuildPalette(CommandContext{Host: h.Host}))
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
	if h == nil {
		return
	}
	if !h.running.CompareAndSwap(false, true) {
		h.Publish(controller.HookCommandResultMsg{
			Gen: h.gen.Load(),
			Err: "A hook command is already running",
		})
		return
	}
	defer h.running.Store(false)

	gen := h.gen.Load()
	if h.Ctrl == nil {
		h.Publish(controller.HookCommandResultMsg{Gen: gen, Err: "hooks are not loaded"})
		return
	}
	mgr := h.Ctrl.Hooks()
	if mgr == nil {
		h.Publish(controller.HookCommandResultMsg{Gen: gen, Err: "hooks are not loaded"})
		return
	}
	res, err := mgr.RunCommand(context.Background(), name, hooks.CommandEvent{
		SessionID: h.Ctrl.SessionID(),
		Cwd:       h.CWD,
		Args:      args,
	})
	if gen != h.gen.Load() {
		return
	}
	if err != nil {
		h.Publish(controller.HookCommandResultMsg{Gen: gen, Err: err.Error()})
		return
	}
	h.Publish(controller.HookCommandResultMsg{
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
	if h == nil || msg.Gen != h.gen.Load() {
		return
	}
	if msg.Err != "" {
		h.Toast.Show(msg.Err, toast.ToastError, 3*time.Second)
		return
	}
	h.applyIntents(msg)
}

func (h *HookCommands) applyIntents(msg controller.HookCommandResultMsg) {
	if msg.StatusSet {
		h.Footer.SetHookStatus(msg.Status)
	}
	if msg.Toast != "" {
		h.Toast.Show(msg.Toast, toast.ToastSuccess, 3*time.Second)
	}
	if msg.List != nil && len(msg.List.Items) > 0 {
		h.pushList(*msg.List)
		return
	}
	if msg.Submit != "" {
		if !h.Submitter.CanSubmit() {
			h.Toast.Show("Cannot submit hook command while a reply is running", toast.ToastWarning, 3*time.Second)
			return
		}
		h.Submitter.Submit(msg.Submit)
	}
}

func (h *HookCommands) pushList(list hooks.CommandList) {
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
				if !h.Submitter.CanSubmit() {
					h.Toast.Show("Cannot submit while a reply is running", toast.ToastWarning, 3*time.Second)
					return
				}
				h.Submitter.Submit(item.Submit)
			},
		})
	}
	if len(cmds) == 0 {
		h.Toast.Show("Hook list had no usable items", toast.ToastWarning, 3*time.Second)
		return
	}
	h.Composer.PushPalette(title, cmds)
}

func keywordsForDetail(detail string) []string {
	if detail == "" {
		return nil
	}
	return []string{detail}
}

package commands

import (
	"time"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/mcp"
)

// mcpDialogTitle heads the /mcp page.
const mcpDialogTitle = "MCP servers — enter toggles"

// MCPCommand returns the /mcp palette page: one row per configured server
// with its live state. Accepting a row flips that server and pushes a fresh
// page, so the dialog doubles as the status view and the switch.
func MCPCommand(
	statuses func() []mcp.ServerStatus,
	toggle func(name string, enabled bool) error,
	push func(title string, cmds []palette.PaletteCommand),
) palette.PaletteCommand {
	return palette.PaletteCommand{
		ID:           "mcp",
		Noun:         "mcp",
		Verb:         "servers",
		Keywords:     []string{"mcp", "server", "enable", "disable", "toggle", "tools"},
		SubmenuTitle: mcpDialogTitle,
		Submenu:      MCPServerRows(statuses, toggle, push),
	}
}

// MCPServerRows builds the /mcp rows: server name, state and the action the
// row performs. statuses is a supplier, not a slice — the page pushed after a
// toggle must show the post-toggle truth. A failed toggle leaves the page as
// it was; the error already surfaced as a toast.
func MCPServerRows(
	statuses func() []mcp.ServerStatus,
	toggle func(name string, enabled bool) error,
	push func(title string, cmds []palette.PaletteCommand),
) []palette.PaletteCommand {
	current := func() []mcp.ServerStatus {
		if statuses != nil {
			return statuses()
		}
		return nil
	}()
	if len(current) == 0 {
		return []palette.PaletteCommand{{
			ID:       "mcp-empty",
			Verb:     "No MCP servers configured — add one in ~/.cozyphi/mcp.json or .mcp.json",
			Disabled: true,
		}}
	}
	rows := make([]palette.PaletteCommand, 0, len(current))
	for _, status := range current {
		name := status.Name
		enable := status.State == mcp.StateDisabled
		action := "disable"
		if enable {
			action = "enable"
		}
		rows = append(rows, palette.PaletteCommand{
			ID:       "mcp-" + name,
			Verb:     name + " — " + mcpStateLabel(status.State) + " — " + action,
			Keywords: []string{name, "mcp", "server", "toggle", action},
			Run: func() {
				if toggle == nil {
					return
				}
				if toggle(name, enable) != nil {
					return
				}
				if push != nil {
					push(mcpDialogTitle, MCPServerRows(statuses, toggle, push))
				}
			},
		})
	}
	return rows
}

// mcpStateLabel names a connection state the way the dialog reads it.
func mcpStateLabel(state mcp.ConnectionState) string {
	switch state {
	case mcp.StateDisabled:
		return "off"
	case mcp.StateConnected:
		return "connected"
	case mcp.StateFailed:
		return "failed"
	default:
		return "configured"
	}
}

// mcpStatusSupplier adapts the host status read for the dialog; no host
// bound means no servers to show (headless callers, tests).
func mcpStatusSupplier(ctx CommandContext) func() []mcp.ServerStatus {
	return hostFn(ctx, func(h Host) func() []mcp.ServerStatus { return h.MCPStatuses })
}

// mcpToggleWithToast keeps the one-toast rule for palette rows, which have
// no error channel: failures surface as a red toast, successes stay silent —
// the refreshed page and the sidebar already show the new state.
func mcpToggleWithToast(ctx CommandContext) func(name string, enabled bool) error {
	toggle := hostFn(ctx, func(h Host) func(string, bool) error { return h.ToggleMCPServer })
	if toggle == nil {
		return nil
	}
	return func(name string, enabled bool) error {
		if err := toggle(name, enabled); err != nil {
			ctx.toast("mcp: "+err.Error(), toast.ToastError, 5*time.Second)
			return err
		}
		return nil
	}
}

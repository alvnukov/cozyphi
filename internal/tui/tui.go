// Package tui is the public entry for the terminal UI shell.
// Implementation lives in subpackages; this facade keeps cmd wiring stable.
package tui

import (
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/shell"
)

// Shell is the TUI root widget.
type Shell = shell.Shell

// Editor is a deprecated alias for Shell.
type Editor = shell.Shell

// CommandRegistry is the slash + palette command catalog.
type CommandRegistry = commands.CommandRegistry

// NewShell builds the interactive TUI root widget.
var NewShell = shell.NewShell

// NewEditor is a deprecated alias for NewShell.
var NewEditor = shell.NewEditor

// NewBuiltinRegistry returns the built-in slash + palette catalog.
var NewBuiltinRegistry = commands.NewBuiltinRegistry

// NewCommandRegistry returns an empty command registry.
var NewCommandRegistry = commands.NewCommandRegistry

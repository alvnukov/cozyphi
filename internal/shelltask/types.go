// Package shelltask owns shell processes across foreground and background turns.
// It persists terminal results before signaling their availability; subscribers
// receive hints and always reconcile against the manager's current state.
package shelltask

import (
	"context"
	"errors"
	"time"

	"github.com/alvnukov/cozyphi/internal/proc"
)

const (
	MaxLive         = 8
	MaxTasks        = 128
	OutputFileBytes = 8 * 1024 * 1024
	TailBytes       = 32 * 1024
)

var (
	ErrClosed   = errors.New("shell tasks are closed")
	ErrNotFound = errors.New("no such shell task")
	ErrFinished = errors.New("shell task already finished")
)

// State describes process completion, independently of foreground/background ownership.
type State string

const (
	// Unknown is a historical observation without a live owner or terminal receipt.
	// Manager never publishes Unknown as a process state.
	Unknown   State = "unknown"
	Running   State = "running"
	Completed State = "completed"
	Failed    State = "failed"
	Stopped   State = "stopped"
)

func (s State) Terminal() bool { return s == Completed || s == Failed || s == Stopped }

// Snapshot is a detached, bounded view; OutputFile contains the retained raw output.
// The pointer fields are honesty rules, not convenience: a process that never ran
// to an exit of its own (still running, or stopped) has no exit code, and a zero
// int there would read as success. Unset times stay nil so JSON omits them — a
// zero time.Time would marshal as 0001-01-01.
type Snapshot struct {
	ID              string     `json:"id"`
	ParentSessionID string     `json:"parent_session_id"`
	ToolUseID       string     `json:"tool_use_id"`
	Command         string     `json:"command"`
	OutputFile      string     `json:"output_file"`
	State           State      `json:"state"`
	Background      bool       `json:"background"`
	Started         time.Time  `json:"started"`
	Finished        *time.Time `json:"finished,omitempty"`
	Deadline        *time.Time `json:"deadline,omitempty"`
	ExitCode        *int       `json:"exit_code,omitempty"`
	Output          string     `json:"output"`
	Truncated       bool       `json:"truncated"`
	Error           string     `json:"error,omitempty"`
}

// Outcome is a terminal receipt scoped to the conversation that launched it.
type Outcome struct {
	Snapshot
	EventID string `json:"event_id"`
}

// Request contains a shell spec already resolved under the tool's permission context.
type Request struct {
	ParentSessionID string
	ToolUseID       string
	Command         string
	Spec            proc.Spec
	Timeout         time.Duration
	Background      bool
}

// Result is either a foreground process result or a background acceptance.
type Result struct {
	Snapshot Snapshot
	Process  proc.Result
}

// Runner is the process seam; production uses proc.Run, tests control its lifecycle.
type Runner func(context.Context, proc.Spec, proc.Limit) (proc.Result, error)

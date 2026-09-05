package job

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/permission"
)

// Status is the lifecycle state of a job.
type Status string

// Job lifecycle status values.
const (
	// StatusStarting means the job was accepted and a goroutine is about to run.
	// There is no wait queue: when MaxConcurrent slots are full, Spawn returns
	// [ErrBusy] instead of enqueueing.
	StatusStarting  Status = "starting"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

// Terminal reports whether no further work will run.
func (s Status) Terminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled, StatusTimedOut:
		return true
	default:
		return false
	}
}

// Meta is persisted as meta.json.
type Meta struct {
	ID              string    `json:"id"`
	ParentID        string    `json:"parent_id,omitempty"`
	ParentToolUseID string    `json:"parent_tool_use_id,omitempty"`
	PreviousJobID   string    `json:"previous_job_id,omitempty"`
	ChildSessionID  string    `json:"child_session_id,omitempty"`
	OutcomeID       string    `json:"outcome_id,omitempty"`
	OutcomeSummary  string    `json:"outcome_summary,omitempty"`
	StopReason      string    `json:"stop_reason,omitempty"`
	UserIntervened  bool      `json:"user_intervened,omitempty"`
	OwnerID         string    `json:"owner_id,omitempty"` // assignment lifetime, independent of conversation history
	ParentDepth     int       `json:"parent_depth"`
	Role            Role      `json:"role,omitempty"`   // explore | worker | review; empty → explore
	Effort          string    `json:"effort,omitempty"` // child-only override; empty inherits the selected model's effort
	Prompt          string    `json:"prompt"`
	Description     string    `json:"description,omitempty"`
	WorkDir         string    `json:"workdir,omitempty"`
	ParentWorkspace string    `json:"parent_workspace,omitempty"`
	Skills          []string  `json:"skills,omitempty"` // canonical names chosen at spawn; rendered into the child prompt
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	FinishedAt      time.Time `json:"finished_at,omitempty"`
	Error           string    `json:"error,omitempty"`
	ResultPath      string    `json:"result_path,omitempty"`
	EventsPath      string    `json:"events_path,omitempty"`
	Dir             string    `json:"dir"`
}

// Info is a list/get snapshot.
type Info struct {
	Meta
}

// SpawnRequest configures a new job.
type SpawnRequest struct {
	Prompt          string
	Description     string
	ParentID        string // parent session or parent job id (opaque to this package)
	OwnerID         string // optional assignment-owner lifetime; never supplied by model arguments
	ParentToolUseID string // originating parent agent tool_use id
	PreviousJobID   string // previous assignment in the same retained child
	Depth           int    // 0 = top-level; tool layer should force Depth for children
	Role            Role   // explore | worker | review; empty → explore
	Effort          string // optional reasoning effort; runner validates against the user-selected model
	WorkDir         string
	// ParentWorkspace is the parent's workspace (usually the session cwd).
	// When set, WorkDir must resolve inside it: the child treats the resolved
	// WorkDir as its write boundary, so an unchecked workdir would widen the
	// parent's boundary. Empty disables confinement (programmatic spawns).
	ParentWorkspace string
	// Skills is the validated, canonical skill set the child runs with.
	// The tool layer decides it; this package only carries it to the runner.
	Skills  []string
	Timeout time.Duration // 0 = no run timeout; Cancel still works
}

// validate rejects structurally invalid spawn requests. When ParentWorkspace
// confines the spawn it also canonicalizes WorkDir in place: the resolved
// absolute path is what Spawn (the only caller) persists, so the runner reads
// one canonical boundary.
func (r *SpawnRequest) validate() error {
	if r.Prompt == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalid)
	}
	if _, err := ParseRole(string(r.Role)); err != nil {
		return err
	}
	if r.ParentWorkspace == "" {
		return nil
	}
	wd, err := resolveWorkDir(r.WorkDir, r.ParentWorkspace)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if !permission.WithinWorkspaceResolved(wd, r.ParentWorkspace) {
		return fmt.Errorf("%w: workdir %q outside parent workspace %q", ErrInvalid, wd, r.ParentWorkspace)
	}
	r.WorkDir = wd
	return nil
}

// resolveWorkDir makes the spawn workdir absolute against the parent
// workspace; an empty workdir means "same as the parent".
func resolveWorkDir(workdir, parentWorkspace string) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		workdir = "."
	}
	return permission.AbsCleanAt(workdir, parentWorkspace)
}

// Event is one JSONL log line.
type Event struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

// WaitResult is returned by Wait / Task.
type WaitResult struct {
	Info    Info
	Summary string   // contents of result.md when present, otherwise the persisted outcome summary
	Outcome *Outcome // complete terminal envelope; nil for jobs without a terminal outcome identity
}

// RecoveryMode controls how [New] treats leftover non-terminal jobs on disk.
type RecoveryMode int

const (
	// RecoverMarkFailed marks starting/running jobs as failed (default).
	// Safe for TUI / CLI restarts so Wait/Cancel do not see zombies.
	RecoverMarkFailed RecoveryMode = iota
	// RecoverIgnore leaves disk state unchanged (tests / manual inspection).
	RecoverIgnore
)

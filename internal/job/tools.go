package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Tool-facing argument shapes. These mirror future agent_* tools but are
// not registered with phi's tool registry yet.

// SpawnArgs is the JSON argument shape for agent_spawn.
type SpawnArgs struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
	WorkDir     string `json:"workdir,omitempty"`
	TimeoutSec  int    `json:"timeout_sec,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	Depth       int    `json:"depth,omitempty"`
	Role        string `json:"role,omitempty"`
}

// WaitArgs is the JSON argument shape for agent_wait.
type WaitArgs struct {
	JobID      string `json:"job_id"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

// CancelArgs is the JSON argument shape for agent_cancel.
type CancelArgs struct {
	JobID string `json:"job_id"`
}

// HandleSpawn is a JSON-tool style entry for agent_spawn.
func (m *Manager) HandleSpawn(ctx context.Context, raw json.RawMessage) (Info, error) {
	var args SpawnArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return Info{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	req := SpawnRequest{
		Prompt:      args.Prompt,
		Description: args.Description,
		WorkDir:     args.WorkDir,
		ParentID:    args.ParentID,
		Depth:       args.Depth,
		Role:        Role(args.Role),
	}
	if args.TimeoutSec > 0 {
		req.Timeout = time.Duration(args.TimeoutSec) * time.Second
	}
	return m.Spawn(ctx, req)
}

// HandleList is a JSON-tool style entry for agent_list.
// Args are ignored; callers filter on Info.Status if needed.
func (m *Manager) HandleList(ctx context.Context, _ json.RawMessage) ([]Info, error) {
	return m.List(ctx)
}

// HandleWait is a JSON-tool style entry for agent_wait.
//
// TimeoutSec limits how long Wait blocks. It does not cancel the job; the
// runner keeps going until Cancel, the job's own TimeoutSec (spawn), or exit.
func (m *Manager) HandleWait(ctx context.Context, raw json.RawMessage) (WaitResult, error) {
	var args WaitArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return WaitResult{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if args.JobID == "" {
		return WaitResult{}, fmt.Errorf("%w: job_id is required", ErrInvalid)
	}
	waitCtx := ctx
	var cancel context.CancelFunc
	if args.TimeoutSec > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(args.TimeoutSec)*time.Second)
		defer cancel()
	}
	return m.Wait(waitCtx, args.JobID)
}

// HandleCancel is a JSON-tool style entry for agent_cancel.
func (m *Manager) HandleCancel(ctx context.Context, raw json.RawMessage) error {
	var args CancelArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if args.JobID == "" {
		return fmt.Errorf("%w: job_id is required", ErrInvalid)
	}
	return m.Cancel(ctx, args.JobID)
}

package job

import (
	"context"
	"fmt"
)

// Task spawns a job and waits for its summary in one blocking call.
// Equivalent to Spawn + Wait. Multiple Task calls can still run in parallel
// via separate goroutines / tool batches at a higher layer.
func (m *Manager) Task(ctx context.Context, req SpawnRequest) (WaitResult, error) {
	info, err := m.Spawn(ctx, req)
	if err != nil {
		return WaitResult{}, err
	}
	res, err := m.Wait(ctx, info.ID)
	if err != nil {
		return WaitResult{}, err
	}
	switch res.Info.Status {
	case StatusFailed:
		return res, fmt.Errorf("job %s failed: %s", res.Info.ID, res.Info.Error)
	case StatusCancelled:
		return res, fmt.Errorf("job %s cancelled: %s", res.Info.ID, res.Info.Error)
	case StatusTimedOut:
		return res, fmt.Errorf("job %s timed out: %s", res.Info.ID, res.Info.Error)
	}
	return res, nil
}

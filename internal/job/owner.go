package job

import "context"

// ListForOwner returns only jobs assigned to ownerID. Empty preserves legacy
// unscoped access, as do the other ForOwner query/action methods.
func (m *Manager) ListForOwner(ctx context.Context, ownerID string) ([]Info, error) {
	infos, err := m.List(ctx)
	if err != nil || ownerID == "" {
		return infos, err
	}
	out := make([]Info, 0, len(infos))
	for _, info := range infos {
		if info.OwnerID == ownerID {
			out = append(out, info)
		}
	}
	return out, nil
}

// WaitForOwner waits only if trusted job metadata belongs to ownerID.
func (m *Manager) WaitForOwner(ctx context.Context, id, ownerID string) (WaitResult, error) {
	if err := m.checkOwner(ctx, id, ownerID); err != nil {
		return WaitResult{}, err
	}
	return m.Wait(ctx, id)
}

// LogForOwner reads logs only if trusted job metadata belongs to ownerID.
func (m *Manager) LogForOwner(ctx context.Context, id string, limit int, ownerID string) ([]Event, error) {
	if err := m.checkOwner(ctx, id, ownerID); err != nil {
		return nil, err
	}
	return m.Log(ctx, id, limit)
}

// CancelForOwner cancels only if trusted job metadata belongs to ownerID.
func (m *Manager) CancelForOwner(ctx context.Context, id, ownerID string) error {
	if err := m.checkOwner(ctx, id, ownerID); err != nil {
		return err
	}
	return m.Cancel(ctx, id)
}

func (m *Manager) checkOwner(ctx context.Context, id, ownerID string) error {
	if ownerID == "" {
		return nil
	}
	info, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	// OwnerID never changes during a job's lifetime. Checking before an action
	// remains safe if the runner finishes and the action loads persisted metadata.
	if info.OwnerID != ownerID {
		return ErrNotFound
	}
	return nil
}

package job

import (
	"context"
	"errors"
	"fmt"
)

// Failed final writes retain the existing job and admission slot, bounding the
// in-memory fallback by MaxConcurrent. There is no retry worker or second queue.
// Queries retry persistence; only durable metadata can become a delivered outcome.
func (m *Manager) reconcileOutcomeLocked(lj *liveJob) error {
	if lj.persistErr == nil {
		return nil
	}
	if err := m.store.writeMeta(lj.meta); err != nil {
		lj.persistErr = err
		return fmt.Errorf("job: undelivered outcome %s: restore writable job storage and retry: %w", lj.meta.ID, err)
	}
	lj.persistErr = nil
	if lj.exited {
		delete(m.jobs, lj.meta.ID)
		<-m.slots
	}
	return nil
}

func (m *Manager) reconcileOutcomes(ctx context.Context, matches func(Meta) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var failures error
	for _, lj := range m.jobs {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		if matches(lj.meta) {
			failures = errors.Join(failures, m.reconcileOutcomeLocked(lj))
		}
	}
	return errors.Join(failures, ctx.Err())
}

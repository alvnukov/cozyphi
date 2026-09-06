package watch

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Observe reports this session's watches for the harness view: whether a
// manager is in force at all, the budgets every watch is held to, and one
// entry per watch this session started.
//
// It reads the manager's own entry list under the manager's lock, and does
// nothing else. No watch is started, stopped, cancelled or subscribed to; no
// command is run; no log is read and no event is published. A watch that has
// reported nothing is reported as one.
//
// A nil manager is the honest answer for a process shape that has none — a
// headless run — and it is not the same as a session that has started no
// watch. The budgets are still reported, because they are compiled in and
// hold wherever a manager would be built.
//
// Nothing that could carry a secret is copied out: not the label, the
// command, the match expression, the working directory, the text of an event
// or the text of an error. What leaves is a shape, a trigger, an interval, a
// count and an outcome.
func Observe(m *Manager) diag.WatchState {
	state := diag.WatchState{
		Known:          true,
		MaxLive:        MaxLive,
		MinInterval:    MinInterval,
		FloodLimit:     FloodLimit,
		FloodWindow:    floodWindow,
		MaxPerDelivery: MaxPerDelivery,
		EventTextLimit: EventTextLimit,
		Watches:        []diag.WatchFacts{},
	}
	if m == nil {
		state.Revision = watchRevision(state)
		return state
	}
	state.Managed = true
	m.mu.Lock()
	for _, id := range m.order {
		e, ok := m.entries[id]
		if !ok {
			continue
		}
		state.Watches = append(state.Watches, diag.WatchFacts{
			Shape:   shapeOf(e.Watch),
			Trigger: triggerOf(e.Watch),
			Every:   e.Every,
			Events:  e.Events,
			Live:    e.Live,
			Outcome: outcomeOf(e),
		})
	}
	m.mu.Unlock()
	state.Revision = watchRevision(state)
	return state
}

// shapeOf is which of the three kinds of watch one is. The two fields that
// decide it are the same two the package doc names, read here rather than
// remembered at start time so the shape cannot disagree with the watch.
func shapeOf(w Watch) diag.WatchShape {
	switch {
	case w.Command == "":
		return diag.WatchTimer
	case w.Every > 0:
		return diag.WatchPoll
	default:
		return diag.WatchStream
	}
}

// triggerOf is what counts as an event from a streaming command. A poll and
// a timer are not triggered by a line at all, and reporting one for them
// would invent a setting the manager does not have.
func triggerOf(w Watch) diag.WatchTrigger {
	if w.Command == "" || w.Every > 0 {
		return ""
	}
	if w.On == OnExit {
		return diag.WatchOnExit
	}
	return diag.WatchOnLine
}

// outcomeOf is what became of one watch, as a category. The three ways a
// watch stops are different problems with different fixes: a command that
// failed, a watch that crossed the session's shared event budget and stopped
// itself, and one that simply finished or was stopped. Which it was is read
// from the manager's own record; why is not, because the error it keeps
// quotes the command that produced it.
func outcomeOf(e *entry) diag.WatchOutcome {
	switch {
	case e.Live:
		return diag.WatchRunning
	case e.flooded:
		return diag.WatchFlooded
	case e.Err != "":
		return diag.WatchFailed
	default:
		return diag.WatchEnded
	}
}

// watchRevision fingerprints what this observation describes: how many
// watches this session started, how many are still running and how many
// events they have produced altogether. Nothing in this package counts these
// — it is only a way to see that two snapshots taken across a start are of
// two different states.
func watchRevision(state diag.WatchState) string {
	live, events := 0, 0
	for _, w := range state.Watches {
		if w.Live {
			live++
		}
		events += w.Events
	}
	return fmt.Sprintf("w%d.l%d.e%d", len(state.Watches), live, events)
}

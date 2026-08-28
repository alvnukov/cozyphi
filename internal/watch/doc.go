// Package watch runs background work that wakes the agent when something
// happens out there: a line in a log, a command that finished, a clock.
//
// A watch is a [Spec] plus a [Source]. Three shapes fall out of the two fields
// that decide it — whether it has a command, and whether it has an interval:
//
//	command, no interval  stream: every matching line is an event, or one
//	                      event when the command exits ([Spec.On])
//	command and interval  poll: run it on each tick, emit when the output
//	                      changes — the shape a remote API needs
//	interval, no command  timer: the label comes back on each tick
//
// Events reach subscribers, never the model directly. What to do with one —
// inject it into a running turn, or start a turn with it — is the session's
// decision, and this package holds no opinion about it.
//
// Two budgets keep a watch from costing more than it is worth: a flood cap
// stops the watch that crosses the session's event budget (one window shared
// by every watch), and a live cap bounds how many run at once. Both fail
// loudly, with a final event saying what happened.
//
// A watch is process-scoped and nothing is persisted: closing cozyphi ends
// every watch. That is the honest contract for a background shell command
// whose parent is gone.
package watch

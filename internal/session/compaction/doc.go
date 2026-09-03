// Package compaction keeps a session's conversation history inside the
// model's context window, in two layers that never touch each other's ground.
//
// The macro cut is durable: it summarizes everything before a cut point into
// a compaction entry appended to the session log, keeping a verbatim tail of
// keepRecentTokens. It rewrites what the session remembers.
//
// The micro projection is provider-view only: under context pressure it
// replaces old oversized tool results — before the current round and outside
// that same verbatim tail — with short metadata stubs on the way to the
// provider, and leaves the log and the transcript exactly as they were. The
// set of stubbed results is frozen between rounds and grows only in batches,
// down to a target that leaves headroom below the trigger, so the prompt
// prefix the provider caches stays stable instead of shifting every round.
package compaction

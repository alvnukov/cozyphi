// Package job is a sub-agent job manager (spawn / list / wait / log / cancel).
//
// Not yet wired into the agent Engine or tool registry. Later integration
// should call [Manager] from tools and supply a [Runner] that drives a child
// Engine.
//
// Disk layout per job:
//
//	<root>/<job-id>/
//	  meta.json      # durable status + spawn metadata
//	  events.jsonl   # append-only log lines
//	  result.md      # final summary for the parent (not the full transcript)
package job

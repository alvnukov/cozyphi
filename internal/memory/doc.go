// Package memory reads and maintains the Claude Code auto-memory corpus shared
// by Claude Code and cozyphi for one repository.
//
// One fact is one Markdown file with YAML frontmatter, written by the agent
// with the ordinary write tool:
//
//	---
//	name: prefers-table-driven-tests
//	description: The user wants new tests written table-driven.
//	metadata:
//	  type: feedback
//	---
//	Write new tests table-driven, one case struct per row.
//
//	**Why:** the repo's existing tests read that way.
//	**How to apply:** ...
//
// The harness keeps Claude Code's MEMORY.md catalog synchronized with topic
// files. It also owns recall — the pass that puts facts relevant to the user's
// prompt into a <system-reminder> on that turn.
package memory

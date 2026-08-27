// Package memory is the agent's durable, project-scoped notebook: facts it
// decided are worth keeping past the end of a session.
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
// The harness owns two things the agent does not: MEMORY.md, the generated
// index that rides in the system prompt, and recall — the pass that puts the
// facts relevant to the user's prompt into a <system-reminder> on that turn.
package memory

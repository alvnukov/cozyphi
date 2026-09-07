// Package webtool is cozyphi's model-facing web research tool: one
// action-dispatch tool with search, fetch, find and read over
// cozy-tools' webfetch and websearch.
//
// The library bounds the network; this package bounds what the network is
// allowed to say to the model. Four layers do that, and they are layers
// rather than a detector on purpose — a classifier that reads the page to
// decide whether the page is lying is asking the attacker to grade their own
// exam.
//
//  1. Visibility budget. fetch returns metadata only, never page text. read
//     and find return fragments the library bounded (4000 bytes by default,
//     20000 at most; 10 matches with 80 characters of context). Links are
//     never followed: every URL is its own call through the permission gate.
//
//  2. Quarantine reader. By default read and find never hand the fragment to
//     the session at all. It goes to a tool-less child model call — the
//     web-reader role — together with the caller's question, and only that
//     child's answer comes back. The reader is handed decoy tools whose
//     definitions are copied from the real bash, write, edit and web tools.
//     A page that talks the reader into calling one has identified itself:
//     the run is aborted, the cached document is flagged
//     injection_suspected:<tool>, the session gets a notice with no page text
//     in it, and further reads of that document are refused unless the user
//     approves a raw one.
//
//  3. Raw escape hatch. raw:true returns the bounded fragment directly, for
//     the times a summary is not good enough — an exact API signature, a
//     code block. The gate asks for it every time, allow-list or not.
//
//  4. Untrusted frame. Every model-facing web text — a reader answer, a raw
//     fragment, search snippets — is wrapped the way internal/agent/outcomes.go
//     wraps child output: a system-reminder around a JSON payload, so no page
//     content can close the wrapper and speak as the harness.
//
// The honest limit: an injection that never touches a decoy can still poison
// the reader's answer. That answer arrives framed and marks the turn tainted,
// which is what the permission layer is for — see permission.TaintGate.
package webtool

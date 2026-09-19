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
//  0. Readiness gate. The tool runs only behind the host's protected-web
//     readiness verdict (Deps.Ready): an explicit web model binding for the
//     quarantined reader. While it is absent every action refuses at the
//     tool entry, before any acquisition or model call, and no permission
//     mode, approval or legacy setting — raw:true, web.quarantine: off, the
//     session model as reader — changes that.
//
//  1. Visibility budget. fetch returns metadata only, never page text. read
//     and find return fragments the library bounded (4000 bytes by default,
//     20000 at most; 10 matches with 80 characters of context). Links are
//     never followed: every URL is its own call through the permission gate.
//
//  2. Quarantine reader. read and find never hand the fragment to the
//     session at all. It goes to a tool-less child model call — the
//     web-reader role — together with the caller's question, and only that
//     child's answer comes back. The reader is handed decoy tools whose
//     definitions are copied from the real bash, write, edit and web tools.
//     A page that talks the reader into calling one has identified itself:
//     the run is aborted, the cached document is flagged
//     injection_suspected:<tool>, the session gets a notice with no page text
//     in it, and every later read of that document is refused — there is no
//     raw read left to approve.
//
//  3. No raw escape hatch. raw:true is refused outright: unchecked page
//     text never reaches the session. Exact text — an API signature, a code
//     block — comes from asking the reader for a verbatim quote.
//
//  4. Untrusted frame. Every model-facing web text — a reader answer,
//     search snippets — is wrapped the way internal/agent/outcomes.go
//     wraps child output: a system-reminder around a JSON payload, so no page
//     content can close the wrapper and speak as the harness.
//
// The honest limit: an injection that never touches a decoy can still poison
// the reader's answer. That answer arrives framed and marks the turn tainted,
// which is what the permission layer is for — see permission.TaintGate.
package webtool

# Session names

A session has one durable title, shared by the saved-session list, retained-session
selector, footer and terminal window title. The terminal follows the selected
session; background work cannot rename the foreground terminal window.

Use `/rename <title>` to name the current conversation manually. A manual title is
pinned: subsequent model attempts cannot overwrite it. Run `/rename` again to
change it yourself. Titles are plain, single-line UTF-8 text, normalized for
spacing, with at most 60 Unicode code points. Empty titles, line breaks, terminal
control characters and unsafe formatting characters are rejected. Do not put
secrets in a title: it is displayed outside the conversation too.

Without an explicit title, displays use a safe excerpt of the first user prompt
(up to 48 Unicode code points), then the short session ID. The saved list retains
the separate last-message preview and session ID; its active marker indicates
file ownership, not which retained session is selected.

## Model naming

The primary model receives `session(action=set_title, title=...)` and an instruction
to name the session once its goal is clear, usually on the first turn. The name
uses 3–7 words in the user's language; substantial topic changes can update it.
There is no extra inference request dedicated to naming. This is a tool call in
the ordinary loop, not a guarantee that every model will name every session.

The tool is absent from child agents and explicitly restricted tool sets. Its
closure owns one session store: switching tabs or replacing the session cannot
redirect an in-flight call. It is exempt from plan-step binding, but still runs
through pre-hooks, the permission gate and post-hooks. The default permission
policy asks before changing the name; normal explicit permission bypass applies.
A manual `/rename` remains pinned even when a model call has been approved.

Successful calls show a compact `Title` row and a `Session named` toast. Replaying
history does not show old toasts. The terminal title tracks the selected session;
restoring a previous external terminal title on exit is not currently supported.

## Log format

Titles are append-only metadata in the session's JSONL log:

```json
{"type":"session_title","id":"entry-id","timestamp":"2026-09-06T09:00:00Z","title":"Investigate session naming","source":"user"}
```

`source` is `user` or `model`. The latest valid title entry wins during replay.
Title entries do not move the conversational leaf or enter model context. A title
is persisted immediately, even before the first assistant reply; a failed write
does not publish a new title. Manual-title protection is checked atomically with
the write. Renaming updates the log's modification time and may change its position
in newest-first session lists.

Existing logs without titles continue to load with the fallback name. Older
binaries that do not recognize `session_title` cannot resume a newly named log.
Persistence otherwise uses the existing session storage guarantees; this is not a
new power-loss durability guarantee.

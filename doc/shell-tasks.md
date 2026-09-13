# Shell tasks

Background Bash runs a command once and delivers its terminal result without
model-issued polling. Use `watch` for a monitor, changing remote status, or a
timer; see [Watches](watch.md).

## Run and inspect

The interactive model can start a command in the background:

```json
{"command":"make build","run_in_background":true}
```

Bash returns a stable task ID and `output_file`. This acknowledges background
execution; it does not claim that the command succeeded. Completion arrives
automatically, including failures and stopped commands. A stopped task reports
no exit code — the process never reached one — and a task without an explicit
timeout carries no deadline; unset facts stay out of the receipt instead of
appearing as a zero that reads as success.

Timeouts are in seconds:

| Launch | Timeout |
| --- | --- |
| Foreground, omitted | 300 seconds |
| Background, omitted | Until completion, stop, or application shutdown |
| Background, explicit | 1–3600 seconds; expiry stops the command |
| Foreground moved to background | The original deadline is preserved |

Moving a foreground command does not restart it or reset its deadline.
Timeouts never silently convert a command into unlimited background work.

Open `/tasks` or `/bashes` to inspect shell tasks across the running application.
These commands show shell work; sub-agents and watches keep their own views.
The list identifies each task's origin and shows running, completed, failed, or
stopped state. `Enter` opens live output, `b` backgrounds the selected foreground
task, and `s` asks to stop it. Confirm with `y`, cancel with `n`; `Esc` or `q`
closes the view. In output, `G` follows the live tail after manual scrolling.
The UI shows a bounded plain-text preview; terminal control sequences do not
execute there, and the raw output file is unchanged.

The `background-shell` keybinding is **Ctrl+B** in the standard and Vim profiles,
and **Shift+F4** in Readline. It is rebindable. If several foreground shell tasks
are eligible, the shortcut opens the list so the user chooses the exact task.
Displayed hints follow the active profile and custom bindings.

The model uses `shell_task` for deliberate inspection or cancellation:

```jsonc
{"action":"list"}
{"action":"get","id":"<task-id>"}
{"action":"stop","id":"<task-id>"}
```

Only tasks belonging to the model's current conversation are accessible through
this tool. It cannot launch commands or background a foreground command.
`get` returns metadata and at most 8000 bytes of the in-memory output tail,
including when workspace-only policy prevents reading the output file.
`stop` requests cancellation; the terminal notification confirms process exit.
Do not repeat `get` or shell commands to wait for completion.

## Ownership and delivery

One application `Runtime` owns the `shelltask.Manager`. A background process
survives the model's final answer, tab changes, and conversation clear or switch.
Its resolved working directory and environment remain those captured at launch,
including when launched from a Git worktree.

The user can inspect the application's tasks with their origins. Model delivery
is narrower: a terminal result belongs only to its original `ParentSessionID`.
Switching conversations does not transfer it to the new conversation. A result
awaits delivery to its owner instead.

The manager records a terminal receipt before signaling availability. Consumers
reconcile current state when signaled; the session acknowledges a receipt only
after recording it. Completion updates the source task's state and reaches the
owning model through the existing event inbox and wake scheduling, without a
polling turn. Automatic delivery remains subject to the session's wake limits.

An event is command output, not human input, an instruction, or permission to
perform another action. Autonomous or unknown turn origins retain existing web
permission restrictions; only explicit user input clears that turn's web taint.
Background launch still passes through the ordinary pre-hook, permission gate,
execution, and post-hook sequence. It uses the Bash deny/default policy without
the one-shot Bash allowlist. A user's background
shortcut changes ownership of an already approved process; it does not execute
another command.

Headless runs and child agents have no background Bash capability or
`shell_task` tool. Their ordinary Bash remains synchronous.

## Output and shutdown

Output is collected while the process runs under the application's jobs directory:
`shell/shells-<random>/sh-<random>/output.log`. Task directories use mode `0700`
and files use `0600`. Use the returned `output_file` path
rather than constructing a storage path. Ordinary `read` on that file obeys the
normal permission policy, including workspace-only reads.

The application permits at most eight live shell tasks and retains at most 128
background task records. Completed foreground tasks do not consume that history.
When space is needed, only the oldest acknowledged terminal background records
may be evicted with their output artifacts; pending results are never evicted.
Artifact cleanup uses one worker with a queue capped at 128 entries. If no safe
eviction or cleanup capacity is available, new background admission or promotion
fails; ordinary foreground execution remains available.

Each background record retains at most a 32 KiB live tail and the first 8 MiB in
its output file; truncation is reported without stopping the command. The full
process output is retained in memory only while executing or returning a native
foreground result. Terminal `result.json` and delivery `ack.json` accompany the
retained background output file.

Pending delivery belongs to the current Runtime and is not recovered
automatically after restart. Delivered results replay from session history;
a historical launch without a known terminal result shows **status unavailable**.
Application restart does not restore commands or restart processes.

Application shutdown cancels and joins managed processes, then removes this
Runtime's private output store. Delivered receipts remain in session history;
output files are available only until history eviction or Runtime shutdown.
An abrupt process crash may leave private artifacts; there is no automatic
recovery or cleanup scan on the next launch. On Unix, cancellation
targets the original process group. Descendants that detach into another
process group or session are outside this cleanup guarantee. Do not rely on this
feature to supervise daemonized processes. Windows uses best-effort tree
termination; full Windows runtime cleanup is not guaranteed.

## Claude Code reference

Checked against official documentation on **2026-09-13**:

- [Background Bash and Ctrl+B](https://code.claude.com/docs/en/interactive-mode#background-bash-commands):
  asynchronous execution, task IDs, output files, and interactive backgrounding.
- [Background command lifetime](https://code.claude.com/docs/en/tools-reference#background-commands):
  Claude Code also supports automatic backgrounding on timeout. CozyPhi keeps
  its explicit timeout as a real limit.
- [Commands](https://code.claude.com/docs/en/commands):
  Claude Code's `/tasks` includes other background work. CozyPhi's view here is
  specifically for shell tasks.
- [Task notifications](https://code.claude.com/docs/en/agent-sdk/typescript#sdktasknotificationmessage)
  and [background-task changes](https://code.claude.com/docs/en/agent-sdk/typescript#sdkbackgroundtaskschangedmessage):
  terminal events and updates delivered without a new human prompt.
- [Monitor](https://code.claude.com/docs/en/tools-reference#monitor-tool):
  background output events. CozyPhi's existing `watch` provides its monitor and
  timer interface.
- [TaskOutput](https://code.claude.com/docs/en/agent-sdk/typescript#taskoutput):
  deprecated in favor of reading the output file. CozyPhi does not add this
  legacy blocking interface.

This is a lifecycle comparison, not a claim of identical UI, persistence,
platform support, or detached-process cleanup.

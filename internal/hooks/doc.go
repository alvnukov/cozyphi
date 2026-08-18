// Package hooks is the policy extension surface for phi tool calls.
//
// Hooks sit beside — not inside — the other extension layers:
//
//   - Skills (internal/llm/skills): prompt knowledge — teach the model how to think.
//   - Gate (internal/permission): host rules + interactive Ask — decide whether
//     the tool may run under workspace policy.
//   - Hooks (this package): user/org policy, audit, and context injection around
//     the tool loop — PreTool before Gate, PostTool after Run.
//   - Tools / Jobs: what the model can invoke.
//
// Configuration is discovered from ~/.phi/hooks and <cwd>/.phi/hooks (see
// doc/hooks.md). It must not be mixed into ~/.phi/config.yaml.
//
// [Manager] fans [Entry] values (Hook + Kind + FailClosed/Async) across the
// tool loop. [Discover] / [Load] build Managers from plugin.json; [CommandHook]
// runs external scripts via stdin/stdout JSON. TUI and `phi run` call [Load]
// at Engine construction.
package hooks

// Package plugin is an in-process event bus for extension points.
//
// It is not the filesystem "plugin.json" loader — that lives in package hooks.
// Declare typed hook points on an Engine (or similar) instance and register
// them in a [Registry]; do not hang them on package-level vars (multi-engine /
// tests will cross-talk).
//
// Shapes:
//
//   - [Hook] — observational: listeners see a message, returns are ignored.
//   - [Chain] — control-plane: handlers return a value reduced in order.
//   - [Registry] — name → Point directory for one host instance.
//
// Subscriptions support [WithPriority] (higher runs first) and [WithOnce].
// Chains default to fail-closed; use [WithFailOpen] to skip handler errors.
//
// Tool-loop policy (Deny / Modify / FailClosed) stays in package hooks; adapt
// external CommandHooks into a Chain or call hooks.Manager from the executor.
package plugin

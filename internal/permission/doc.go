// Package permission implements the tool-execution gate: policy modes,
// decisions, and the user approval flow.
//
// # Containment and TOCTOU
//
// Resolving targets at the gate closes the lexical gap: an approved path and
// its physical target must satisfy the same rules, so a leaf or ancestor
// symlink leading outside the workspace or into a sensitive path fails closed.
// A window remains between this check and the mutation itself, where a local
// racing process can swap a component after approval; write and edit close it
// by asking the gate again from inside the mutation, once before any directory
// is created and once immediately before the rename, so a component swapped in
// the meantime is judged on its new physical target. What is left is the
// check-then-act floor of the two syscalls before the rename, which only
// descriptor-relative (openat-style) mutations remove; inside it the leaf is
// still safe because a rename never follows a symlink.
//
// Platforms: EvalSymlinks behaves the same on all supported platforms; on
// macOS directories such as /tmp resolve to /private/tmp, so workspace roots
// are resolved once at gate construction to keep both sides of the comparison
// in physical form.
package permission

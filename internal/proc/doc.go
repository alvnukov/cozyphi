// Package proc owns external subprocess lifetime: bounded command runs and
// long-lived protocol processes. It starts argv without a shell, bounds output,
// terminates whole process trees, and always reaps with Wait.
//
// Run is for finite commands with bounded combined output, RunSplit for the
// finite commands whose stdout is data and whose stderr is only a log. Start is
// for long-lived framed protocols whose caller reads stdout and writes stdin;
// stdout and stderr are never mixed. The supplied lifetime context owns the
// process: canceling it kills the tree promptly, while Close applies a
// caller-provided grace deadline.
//
// Content-Length framing, JSON-RPC routing, and shell semantics stay out of
// this module.
package proc

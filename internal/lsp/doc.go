// Package lsp hides the gopls lifecycle behind one model-facing query seam.
// Assembly owns one Manager per workspace process; Engines receive only a
// borrowed QueryFunc, so they can neither start nor stop the server.
//
// The wire protocol is Content-Length framed JSON-RPC over a long-lived
// subprocess started through internal/proc. Only normalized, bounded,
// workspace-contained results escape this package: raw URIs, JSON payloads,
// PIDs, argv, env, and stderr never reach callers.
package lsp

package lsp

import "time"

// Baseline resource limits are part of the tracer, not deferred hardening.
const (
	// MaxHeaderBytes bounds a single Content-Length header before allocation.
	MaxHeaderBytes = 8 * 1024
	// MaxFrameBytes bounds a single frame body before allocation.
	MaxFrameBytes = 8 * 1024 * 1024
	// MaxFileBytes bounds a document snapshot read for sync and positions.
	MaxFileBytes = 4 * 1024 * 1024
	// MaxTextFieldBytes bounds any normalized text field (hover, message).
	MaxTextFieldBytes = 8 * 1024
	// MaxStderrTailBytes bounds the retained subprocess stderr tail.
	MaxStderrTailBytes = 64 * 1024
	// DefaultItemLimit is the result count when Query.Limit is unset.
	DefaultItemLimit = 50
	// MaxItemLimit is the hard result count ceiling.
	MaxItemLimit = 200
	// MaxOutputBytes bounds the final rendered tool output.
	MaxOutputBytes = 50 * 1024
	// MaxOpenDocuments bounds simultaneously open documents per client.
	MaxOpenDocuments = 32
	// MaxOpenTextBytes bounds total synchronized document text per client.
	MaxOpenTextBytes = 32 * 1024 * 1024
	// MaxDiagCacheDocs bounds cached diagnostic documents per client.
	MaxDiagCacheDocs = 256
	// MaxConfigBytes bounds the owner-controlled lsp.json file.
	MaxConfigBytes = 64 * 1024
)

// shutdownGrace is the bounded graceful-shutdown window applied before the
// process tree is killed and reaped.
const shutdownGrace = 2 * time.Second

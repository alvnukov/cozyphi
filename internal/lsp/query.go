package lsp

import (
	"context"
	"errors"
	"fmt"
)

// Operation names the one model-facing tool's fixed operation set.
type Operation string

// The approved V1 operations. Only exact-position definition is implemented in
// this package today; the others return a typed unsupported error until their
// tickets land behind the same frozen contract.
const (
	OpLanguages   Operation = "languages"
	OpDefinition  Operation = "definition"
	OpReferences  Operation = "references"
	OpHover       Operation = "hover"
	OpSymbols     Operation = "symbols"
	OpCalls       Operation = "calls"
	OpDiagnostics Operation = "diagnostics"
)

// Direction selects a call-hierarchy traversal; calls requires exactly one.
type Direction string

const (
	DirectionIncoming Direction = "incoming"
	DirectionOutgoing Direction = "outgoing"
)

// Query is the frozen normalized input to Manager.Query. File is an absolute,
// cleaned path already resolved by the tool adapter against its engine cwd;
// the Manager re-validates physical containment. Line and Character are
// 1-based Unicode code-point positions. Limit defaults to DefaultItemLimit and
// is clamped to MaxItemLimit.
type Query struct {
	Op                 Operation
	File               string
	Symbol             string
	Line               int
	Character          int
	Query              string
	Direction          Direction
	IncludeDeclaration bool
	Limit              int
}

// QueryFunc is the borrowed capability handed to Engines. It never exposes
// lifecycle authority.
type QueryFunc func(context.Context, Query) (Result, error)

// Result is the bounded normalized output of one query. Exactly one variant is
// populated per operation; the tool renderer selects it from the query's Op.
type Result struct {
	Locations   []Location   `json:"locations,omitempty"`
	Hover       *Hover       `json:"hover,omitempty"`
	Symbols     []Symbol     `json:"symbols,omitempty"`
	Calls       []Call       `json:"calls,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Languages   []Language   `json:"languages,omitempty"`

	// Truncated reports that item or field limits were applied.
	Truncated bool `json:"truncated,omitempty"`
	// Omitted counts normalized results dropped by the item limit.
	Omitted int `json:"omitted,omitempty"`
	// Warnings carry bounded, sanitized notices (never raw protocol data).
	Warnings []string `json:"warnings,omitempty"`
}

// Location is a workspace-relative, 1-based, end-exclusive source position.
type Location struct {
	File         string `json:"file"`
	Line         int    `json:"line"`
	Character    int    `json:"character"`
	EndLine      int    `json:"endLine"`
	EndCharacter int    `json:"endCharacter"`
}

// Hover is bounded markdown/plaintext with an optional 1-based range.
type Hover struct {
	Text         string `json:"text"`
	Line         int    `json:"line,omitempty"`
	Character    int    `json:"character,omitempty"`
	EndLine      int    `json:"endLine,omitempty"`
	EndCharacter int    `json:"endCharacter,omitempty"`
}

// Symbol is a bounded, stable-kind navigation symbol.
type Symbol struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Detail    string   `json:"detail,omitempty"`
	Container string   `json:"container,omitempty"`
	Location  Location `json:"location"`
}

// Call is one call-hierarchy edge with a from/to symbol and a call site.
type Call struct {
	From     Symbol   `json:"from"`
	To       Symbol   `json:"to"`
	Location Location `json:"location"`
}

// Diagnostic is one bounded, workspace-contained diagnostic entry.
type Diagnostic struct {
	Severity     string `json:"severity"`
	Code         string `json:"code,omitempty"`
	Source       string `json:"source,omitempty"`
	Message      string `json:"message"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	Character    int    `json:"character"`
	EndLine      int    `json:"endLine"`
	EndCharacter int    `json:"endCharacter"`
}

// Language is one bounded language-server status record.
type Language struct {
	Language    string   `json:"language"`
	Server      string   `json:"server"`
	Configured  bool     `json:"configured"`
	Installed   bool     `json:"installed"`
	Running     bool     `json:"running"`
	ActiveRoots int      `json:"activeRoots"`
	Error       string   `json:"error,omitempty"`
	Operations  []string `json:"operations,omitempty"`
	InstallHint string   `json:"installHint,omitempty"`
}

// ErrorKind names the frozen typed error categories.
type ErrorKind string

const (
	ErrInvalid     ErrorKind = "invalid"
	ErrAmbiguous   ErrorKind = "ambiguous"
	ErrUnsupported ErrorKind = "unsupported"
	ErrUnavailable ErrorKind = "unavailable"
	ErrProtocol    ErrorKind = "protocol"
	ErrClosed      ErrorKind = "closed"
)

// Error is a typed LSP error. Callers distinguish categories with errors.As.
type Error struct {
	Kind    ErrorKind
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return "lsp: <nil>"
	}
	return "lsp: " + string(e.Kind) + ": " + e.Message
}

// As reports whether target's kind matches e's.
func (e *Error) As(target any) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	t.Kind = e.Kind
	t.Message = e.Message
	return true
}

func newError(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// errKind extracts an ErrorKind from err, defaulting to protocol for wrapped
// non-LSP errors. Context cancellation remains discoverable via errors.Is.
func errKind(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ErrProtocol
}

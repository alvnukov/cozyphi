package diag

import (
	"strconv"
	"strings"
	"time"
)

// Audit actions, one per entry point.
const (
	AuditCatalog  = "catalog"
	AuditSnapshot = "snapshot"
	AuditExplain  = "explain"
)

// AuditResult is how one request ended. It is a word from a closed set
// rather than a message: an error's text is arbitrary and may quote whatever
// the owner was holding, so what happened travels and why never does.
type AuditResult string

// AuditResult values.
const (
	// AuditAnswered means an answer was produced. It may still have been
	// partial or truncated — those are their own fields, because an answer
	// that lost a category is not the same event as one that lost nothing.
	AuditAnswered AuditResult = "answered"
	// AuditRefused means the request was turned down before anything was
	// observed: an unknown category, an unknown key, a category with no
	// collector. Nothing was read, so nothing could have leaked.
	AuditRefused AuditResult = "refused"
	// AuditFailed means an owner could not be observed and the request had
	// nothing to return. Why is not recorded.
	AuditFailed AuditResult = "failed"
	// AuditCanceled means the caller's context ended first — an aborted
	// turn, usually. It is deliberately not "failed": nothing went wrong.
	AuditCanceled AuditResult = "canceled"
)

// AuditEvent is the record one harness request leaves behind. It is an
// allowlist by construction, and the epic's line is drawn here: what was
// asked, how far it reached, and how it ended — never what came back.
//
// There is no member for a value, a source ref, a reason, an error message
// or a credential to land in. Category and Key are the addresses the request
// named, and Key has been through the same sanitizing and bounding as any
// value before it arrives, because the model supplies it.
type AuditEvent struct {
	// Action is catalog, snapshot or explain.
	Action string
	// Category is the category asked for. Empty is the overview.
	Category Category
	// Key is the field asked for, on explain. Empty otherwise.
	Key string
	// Mode is the snapshot shape — overview or detail. Empty on the other
	// two actions, which have only one shape each.
	Mode string
	// Result is how the request ended.
	Result AuditResult
	// Partial is whether a category could not be observed.
	Partial bool
	// Truncated is whether the size budget dropped anything.
	Truncated bool
	// Categories and Fields are how much was answered: the counts, never
	// the contents.
	Categories int
	Fields     int
	// Elapsed is how long answering took, so a budget that fired is visible
	// as the wait it really was.
	Elapsed time.Duration
}

// Line renders the event as one log line. It is a convenience for whoever
// wires the sink — the whole event is already safe, so this only decides the
// spelling.
func (e AuditEvent) Line() string {
	var b strings.Builder
	b.WriteString("harness ")
	b.WriteString(e.Action)
	if e.Category != "" {
		b.WriteString(" category=")
		b.WriteString(string(e.Category))
	}
	if e.Key != "" {
		b.WriteString(" key=")
		b.WriteString(e.Key)
	}
	if e.Mode != "" {
		b.WriteString(" mode=")
		b.WriteString(e.Mode)
	}
	b.WriteString(" result=")
	b.WriteString(string(e.Result))
	b.WriteString(" partial=")
	b.WriteString(strconv.FormatBool(e.Partial))
	b.WriteString(" truncated=")
	b.WriteString(strconv.FormatBool(e.Truncated))
	b.WriteString(" categories=")
	b.WriteString(strconv.Itoa(e.Categories))
	b.WriteString(" fields=")
	b.WriteString(strconv.Itoa(e.Fields))
	b.WriteString(" elapsed=")
	b.WriteString(e.Elapsed.Round(time.Millisecond).String())
	return b.String()
}

// audit hands one event to the sink, if there is one. Every entry point goes
// through here rather than calling the sink itself, so a request that ends
// early still leaves a record and a nil sink stays free.
func (r *Registry) audit(event AuditEvent) {
	if r == nil || r.sink == nil {
		return
	}
	r.sink(event)
}

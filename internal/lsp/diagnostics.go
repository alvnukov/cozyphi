package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// diagnosticsWait bounds how long a diagnostics query waits for confirmed or
// unconfirmed data before reporting pending. It is a var so tests can prove
// the pending path quickly; the user-facing value is frozen at five seconds.
var diagnosticsWait = 5 * time.Second

// pushDiags is the publishDiagnostics state for one URI. Versioned entries
// are proven current when their version matches the synced document; an
// unversioned publication can never prove freshness and stays unconfirmed.
type pushDiags struct {
	versioned   []Diagnostic
	versionedAt int // document version the versioned slice belongs to; -1 = none
	unconfirmed []Diagnostic
	hasUnconf   bool
	unconfStamp int64 // arrival stamp of the unconfirmed class; 0 = none yet
	stamp       int64 // arrival order for bounded-cache eviction
}

// pullDiags is the textDocument/diagnostic report state for one URI.
type pullDiags struct {
	items    []Diagnostic
	resultID string
}

// mergedDiags is the last confirmed merged result for one URI, keyed by the
// document version and content hash it was computed against.
type mergedDiags struct {
	version   int
	hash      string
	diags     []Diagnostic
	truncated bool
	omitted   int
}

// diagCache holds push, pull, and merged diagnostics per URI for one client
// generation. A restart builds a fresh client with empty caches, so stale
// generations can never surface.
type diagCache struct {
	mu     sync.Mutex
	push   map[string]*pushDiags
	pull   map[string]*pullDiags
	merged map[string]*mergedDiags
	// signal is closed and replaced on every accepted publication so waiters
	// re-evaluate without polling.
	signal chan struct{}
	stamp  int64
}

func newDiagCache() *diagCache {
	return &diagCache{
		push:   make(map[string]*pushDiags),
		pull:   make(map[string]*pullDiags),
		merged: make(map[string]*mergedDiags),
		signal: make(chan struct{}),
	}
}

// current returns the broadcast channel to wait on for new publications.
func (d *diagCache) current() <-chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.signal
}

// stampNow returns the current arrival stamp. Diagnostics queries capture it
// before syncing so unconfirmed publications that predate the sync cannot
// speak for the freshly synchronized document version.
func (d *diagCache) stampNow() int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stamp
}

func (d *diagCache) broadcast() {
	close(d.signal)
	d.signal = make(chan struct{})
}

// publish ingests one publishDiagnostics notification. Related documents
// pass URI normalization and workspace containment before caching. Versioned
// publications are accepted only when they match the currently synced
// document version; older or mismatched versions are ignored. Empty
// publications clear only their own cache class.
func (d *diagCache) publish(workspace string, docs *docStore, params json.RawMessage) {
	var p struct {
		URI         string           `json:"uri"`
		Version     *int             `json:"version"`
		Diagnostics []wireDiagnostic `json:"diagnostics"`
	}
	if json.Unmarshal(params, &p) != nil || p.URI == "" {
		return
	}
	path, err := pathFromURI(p.URI)
	if err != nil {
		return
	}
	inside, err := contained(workspace, path)
	if err != nil || !inside {
		return
	}
	diags, err := normalizeDiagnostics(workspace, p.URI, p.Diagnostics)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.stamp++
	entry := d.push[p.URI]
	if entry == nil {
		entry = &pushDiags{versionedAt: -1}
		d.push[p.URI] = entry
	}
	entry.stamp = d.stamp
	switch p.Version {
	case nil:
		// Unversioned data can never prove freshness; retain it unconfirmed.
		entry.unconfirmed, entry.hasUnconf, entry.unconfStamp = diags, true, d.stamp
	default:
		current, ok := docs.currentVersion(p.URI)
		if !ok || *p.Version != current {
			return // older or mismatched version: ignored entirely
		}
		entry.versioned, entry.versionedAt = diags, current
	}
	d.evictLocked()
	d.broadcast()
}

// evictLocked bounds the push cache by dropping the least recently published
// URI.
func (d *diagCache) evictLocked() {
	for len(d.push) > MaxDiagCacheDocs {
		var oldest string
		for uri, e := range d.push {
			if oldest == "" || e.stamp < d.push[oldest].stamp {
				oldest = uri
			}
		}
		if oldest == "" {
			return
		}
		delete(d.push, oldest)
	}
}

// storePull ingests one textDocument/diagnostic report. A full report
// replaces the pull entries for the documents it mentions; an unchanged
// report keeps the previous items and only rotates the result id.
func (d *diagCache) storePull(workspace, uri string, report json.RawMessage) error {
	var r struct {
		Kind     string `json:"kind"`
		ResultID string `json:"resultId"`
		Items    []struct {
			URI         string           `json:"uri"`
			Diagnostics []wireDiagnostic `json:"diagnostics"`
		} `json:"items"`
	}
	if len(report) == 0 || string(report) == "null" {
		return newError(ErrProtocol, "diagnostics: null diagnostic report")
	}
	if err := json.Unmarshal(report, &r); err != nil {
		return newError(ErrProtocol, "diagnostics: %v", err)
	}
	if r.Kind != "full" && r.Kind != "unchanged" {
		return newError(ErrProtocol, "diagnostics: unknown report kind %q", r.Kind)
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if r.Kind == "unchanged" {
		if entry := d.pull[uri]; entry != nil {
			entry.resultID = r.ResultID
		}
		return nil
	}
	for _, item := range r.Items {
		path, err := pathFromURI(item.URI)
		if err != nil {
			return newError(ErrProtocol, "diagnostics: %v", err)
		}
		inside, err := contained(workspace, path)
		if err != nil {
			return newError(ErrProtocol, "diagnostics: %v", err)
		}
		if !inside {
			continue
		}
		diags, err := normalizeDiagnostics(workspace, item.URI, item.Diagnostics)
		if err != nil {
			return err
		}
		entry := d.pull[item.URI]
		if entry == nil {
			entry = &pullDiags{}
			d.pull[item.URI] = entry
		}
		entry.items = diags
		if item.URI == uri {
			entry.resultID = r.ResultID
		}
	}
	if _, ok := d.pull[uri]; !ok {
		// A full report without an item for the requested document means the
		// document is diagnostics-free: record the explicit empty state.
		d.pull[uri] = &pullDiags{resultID: r.ResultID}
	}
	return nil
}

// previousResultID returns the stored pull result id for uri, if any.
func (d *diagCache) previousResultID(uri string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e := d.pull[uri]; e != nil {
		return e.resultID
	}
	return ""
}

// snapshotState copies the cached state needed to evaluate one query.
// minStamp drops unconfirmed publications that arrived before the query
// began syncing this document version; versioned entries are version-gated
// instead.
func (d *diagCache) snapshotState(
	uri string,
	version int,
	minStamp int64,
) (versioned []Diagnostic, versionedOK bool, unconfirmed []Diagnostic, unconfOK bool, pullItems []Diagnostic) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e := d.push[uri]; e != nil {
		if e.versionedAt == version {
			versioned, versionedOK = e.versioned, true
		}
		if e.hasUnconf && e.unconfStamp > minStamp {
			unconfirmed, unconfOK = e.unconfirmed, true
		}
	}
	if e := d.pull[uri]; e != nil {
		pullItems = e.items
	}
	return
}

// mergedResult returns the cached merged result for an unchanged snapshot.
func (d *diagCache) mergedResult(uri string, version int, hash string) (Result, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e := d.merged[uri]
	if e == nil || e.version != version || e.hash != hash {
		return Result{}, false
	}
	return Result{
		Diagnostics: e.diags,
		Status:      StatusCached,
		Truncated:   e.truncated,
		Omitted:     e.omitted,
	}, true
}

// storeMerged records the confirmed merged result for one snapshot.
func (d *diagCache) storeMerged(
	uri string,
	version int,
	hash string,
	bounded []Diagnostic,
	truncated bool,
	omitted int,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.merged[uri] = &mergedDiags{
		version:   version,
		hash:      hash,
		diags:     bounded,
		truncated: truncated,
		omitted:   omitted,
	}
}

// diagnostics implements the frozen diagnostics handler. The sync barrier
// always runs first; then pull (when advertised) and the push caches are
// merged only for entries proven current for the synchronized snapshot.
// Results report fresh, cached, unconfirmed, or pending — never a false
// empty-success claim.
func (*Manager) diagnostics(ctx context.Context, c *client, q Query) (Result, error) {
	// Capture the arrival stamp before the sync barrier: publications landing
	// during the wait may speak for this version, older ones may not.
	syncStamp := c.diag.stampNow()
	snap, err := c.syncDocument(ctx, q.File)
	if err != nil {
		return Result{}, err
	}

	// An unchanged snapshot may reuse the last confirmed merged result.
	if !snap.changed {
		if res, ok := c.diag.mergedResult(snap.uri, snap.version, snap.hash); ok {
			return res, nil
		}
	}

	pulled := false
	if c.supports("diagnosticProvider") {
		params := map[string]any{"textDocument": map[string]any{"uri": snap.uri}}
		if id := c.diag.previousResultID(snap.uri); id != "" {
			params["previousResultId"] = id
		}
		raw, err := c.request(ctx, "textDocument/diagnostic", params)
		if err != nil {
			return Result{}, requestError(ctx, q.Op, err)
		}
		if err := c.diag.storePull(c.workspace, snap.uri, raw); err != nil {
			return Result{}, err
		}
		pulled = true
	}

	deadline := time.After(diagnosticsWait)
	for {
		if res, ok := evaluateDiagnostics(c, snap, q.Limit, pulled, syncStamp); ok {
			return res, nil
		}
		select {
		case <-c.diag.current():
		case <-deadline:
			return Result{Status: StatusPending}, nil
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-c.done:
			return Result{}, requestError(ctx, q.Op, c.failure())
		}
	}
}

// evaluateDiagnostics merges proven-current sources for the snapshot. ok=false
// means no confirmed or unconfirmed data exists yet and the caller should
// keep waiting. Precedence: fresh (pull or matching-version push, merged),
// then unconfirmed, then pending. Unconfirmed publications carry no version:
// only ones that arrived after this query began syncing the new document
// version may satisfy it; on an unchanged snapshot all of them may.
func evaluateDiagnostics(
	c *client,
	snap docSnapshot,
	limit int,
	pulled bool,
	syncStamp int64,
) (Result, bool) {
	var minStamp int64
	if snap.changed {
		minStamp = syncStamp
	}
	versioned, versionedOK, unconfirmed, unconfOK, pullItems := c.diag.snapshotState(snap.uri, snap.version, minStamp)
	if pulled || versionedOK {
		// Pull does not erase unrelated current versioned push entries: both
		// proven-current sources merge and deduplicate by identity.
		items := make([]Diagnostic, 0, len(pullItems)+len(versioned))
		items = append(items, pullItems...)
		items = append(items, versioned...)
		bounded, omitted := finalize(items, limit, compareDiagnostic)
		res := Result{
			Diagnostics: bounded,
			Status:      StatusFresh,
			Omitted:     omitted,
			Truncated:   omitted > 0,
		}
		c.diag.storeMerged(snap.uri, snap.version, snap.hash, bounded, res.Truncated, omitted)
		return res, true
	}
	if unconfOK {
		bounded, omitted := finalize(append([]Diagnostic{}, unconfirmed...), limit, compareDiagnostic)
		return Result{
			Diagnostics: bounded,
			Status:      StatusUnconfirmed,
			Omitted:     omitted,
			Truncated:   omitted > 0,
			Warnings:    []string{"unconfirmed: the server published diagnostics without a document version"},
		}, true
	}
	return Result{}, false
}

// normalizeDiagnostics bounds and locates a wire diagnostic batch. Items the
// locator cannot place are dropped: containment failures are never cached.
func normalizeDiagnostics(workspace, uri string, batch []wireDiagnostic) ([]Diagnostic, error) {
	out := make([]Diagnostic, 0, len(batch))
	for _, wd := range batch {
		loc, _, ok, err := locate(workspace, OpDiagnostics, wireLocation{URI: uri, Range: wd.Range})
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		message, _ := boundText(wd.Message)
		source, _ := boundText(wd.Source)
		code, _ := boundText(formatDiagnosticCode(wd.Code))
		out = append(out, Diagnostic{
			Severity:     severityName(wd.Severity),
			Code:         code,
			Source:       source,
			Message:      message,
			File:         loc.File,
			Line:         loc.Line,
			Character:    loc.Character,
			EndLine:      loc.EndLine,
			EndCharacter: loc.EndCharacter,
		})
	}
	return out, nil
}

// severityName maps the LSP severity numbers to stable names; unknown values
// stay visible instead of being silently coerced.
func severityName(n int) string {
	switch n {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "information"
	case 4:
		return "hint"
	}
	return fmt.Sprintf("unknown:%d", n)
}

// formatDiagnosticCode renders the wire code, which may be a string or a
// number, as one bounded string.
func formatDiagnosticCode(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return ""
	}
}

package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

// ---- tooldef.Tool constructor ----

var editDescription = `Edit a file using the whole-file TAG and LINE#HASH anchors printed by the latest
read with mode:"edit", editable grep, edit or write result for that file.
View reads never authorize edits.

Required hash: the 4 hex chars AFTER # in the @file path#TAG header
(e.g. A1B2 from "@file src/app.py#A1B2") — not "@file", not the path, not the #.
Put multiple changes to the same file in one edits array — they share one TAG,
apply against the same snapshot, and must all come from one observation of it.
A failed edit keeps the authorization: fix the call and retry without re-reading.
A successful edit replaces it, printing the new TAG and live anchors that
authorize the next edit; any other line needs a fresh read with mode:"edit" first.
On an [edit:<code>] refusal follow its message; never resend the same call unchanged.

Each element of edits is a range replace:
- from + to (LINE#HASH only, e.g. "5#abc" — do not include |content) + content
- content: string (use \n for multiple lines); omit or null to delete lines
- to insert after a line, replace that line with itself plus the new lines
- to insert before a line, replace that line with the new lines plus itself

For creating a new file or replacing a whole file, use write instead.

Examples:
{"path":"src/app.py","hash":"A1B2","edits":[{"from":"5#abc","to":"8#def","content":"  combined = True"}]}
{"path":"src/app.py","hash":"A1B2","edits":[{"from":"3#ghi","to":"3#ghi","content":"  x = 1\n  # new comment"}]}`

// EditTool returns the edit (hashline) tool definition + handler. An optional
// ledger lets a session registry share authorization with editable reads.
func EditTool(ledgers ...*editledger.Ledger) tooldef.Tool {
	var ledger *editledger.Ledger
	if len(ledgers) > 0 {
		ledger = ledgers[0]
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "edit",
			Description: editDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "File to edit; use the same path passed to read.",
					},
					"hash": llm.Object{
						"type":        "string",
						"description": "4 hex chars after # in @file path#TAG (e.g. A1B2). No @file, no #, no path.",
					},
					"edits": llm.Object{
						"type":        "array",
						"description": "Edits in document order against the same original snapshot.",
						"items": llm.Object{
							"type": "object",
							"properties": llm.Object{
								"content": llm.Object{
									"type":        "string",
									"description": "Replacement lines (use \\n for multiple lines). Omit to delete the range.",
								},
								"from": llm.Object{
									"type":        "string",
									"description": "LINE#HASH for range start (e.g. 5#abc). Do not include |content.",
								},
								"to": llm.Object{
									"type":        "string",
									"description": "LINE#HASH for range end inclusive (e.g. 8#def). Do not include |content.",
								},
							},
							"required":             []string{"from", "to"},
							"additionalProperties": true,
						},
					},
				},
				Required: []string{"path", "hash", "edits"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in EditInput
			_ = json.Unmarshal(input, &in)
			// The diff card row shows path + stats; an edit count next to the
			// path would only restate what the stats say better.
			return strings.TrimSpace(in.Path)
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			return runAuthorizedEdit(ctx, input, ledger)
		},
	}
}

// ---- Wire types ----

// EditInput is the edit tool payload (path + file TAG + flat edits).
type EditInput struct {
	Path  string     `json:"path"`
	Hash  string     `json:"hash"`
	Edits []FlatEdit `json:"edits"`
}

// FlatEdit is the wire shape for each element in "edits".
type FlatEdit struct {
	Content *string `json:"content,omitempty"`
	From    string  `json:"from,omitempty"`
	To      string  `json:"to,omitempty"`
}

// ---- Internal parsed types ----

// LineRef is a parsed LINE#HASH reference.
type LineRef struct {
	Line int
	Hash string
}

// ParsedRef is a start+end pair of line references.
type ParsedRef struct {
	Start LineRef
	End   LineRef
}

// ParsedEdit is a fully parsed single edit.
type ParsedEdit struct {
	Spec ParsedRef
	Dst  []string
}

// Annotated pairs a parsed edit with origin metadata for sorting.
type Annotated struct {
	edit     ParsedEdit
	index    int
	sortLine int
}

// HashMismatch records a line whose hash changed.
type HashMismatch struct {
	Line     int
	Expected string
	Actual   string
}

// HashlineMismatchError is returned when hashes don't match (file changed).
type HashlineMismatchError struct {
	mismatches []HashMismatch
	fileLines  []string
	msg        string
}

func (e *HashlineMismatchError) Error() string { return e.msg }

// EditRefusal is a typed edit refusal: a stable wire code the analyzer and
// telemetry count on, one sentence of what happened, and exactly one next
// step. It never carries the call's arguments or replacement content.
type EditRefusal struct {
	Code string
	What string
	Next string
}

func (e *EditRefusal) Error() string {
	return fmt.Sprintf("[edit:%s] %s. Do not retry the same call unchanged. %s", e.Code, e.What, e.Next)
}

// ---- Main entry points ----

func runEdit(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	param, err := parseEditInput(ctx, input)
	if err != nil {
		return tooldef.Result{}, err
	}
	return runParsedEdit(ctx, param, nil, nil, nil)
}

func runAuthorizedEdit(ctx context.Context, input json.RawMessage, ledger *editledger.Ledger) (tooldef.Result, error) {
	param, err := parseEditInput(ctx, input)
	if err != nil {
		return tooldef.Result{}, err
	}
	if len(param.Edits) == 0 {
		return tooldef.Result{}, &EditRefusal{
			Code: "invalid_ref",
			What: "edits must be a non-empty array of {from, to} ranges",
			Next: "retry with at least one edit against the read snapshot",
		}
	}
	if normalizeFileTag(param.Hash) == "" {
		return tooldef.Result{}, &EditRefusal{
			Code: "invalid_ref",
			What: "edit requires hash: the 4 hex chars after # in the @file path#TAG header from read/grep (e.g. A1B2)",
			Next: "copy the TAG from the read header into the hash argument and retry",
		}
	}
	// Parse the anchors before claiming so a malformed reference reports its
	// own code instead of masquerading as an authorization problem.
	parsed := make([]ParsedEdit, len(param.Edits))
	refs := make([]editledger.Ref, 0, len(param.Edits)*2)
	for i, edit := range param.Edits {
		pe, err := edit.toParsedEdit()
		if err != nil {
			return tooldef.Result{}, &EditRefusal{
				Code: "invalid_ref",
				What: fmt.Sprintf("edits[%d]: %s", i, err),
				Next: `retry with from/to as LINE#HASH anchors exactly as the read returned (e.g. "5#abc")`,
			}
		}
		parsed[i] = pe
		refs = append(refs,
			editledger.Ref{Line: pe.Spec.Start.Line, Hash: pe.Spec.Start.Hash},
			editledger.Ref{Line: pe.Spec.End.Line, Hash: pe.Spec.End.Hash},
		)
	}
	claim, resolution := ledger.Claim(param.Path, normalizeFileTag(param.Hash), refs)
	if resolution.Outcome.Refused() {
		return tooldef.Result{}, refusalForOutcome(
			resolution.Outcome,
			tooldef.RelToCwd(ctx, param.Path),
			normalizeFileTag(param.Hash),
		)
	}
	notices := rebaseEdits(param, parsed, resolution)
	// runParsedEdit owns the claim's lifecycle: it settles the claim with a
	// successor grant when the edit applies and hands it back on any failure,
	// so the ordering cannot drift between callers.
	return runParsedEdit(ctx, param, notices, ledger, claim)
}

// rebaseEdits rewrites the claimed line numbers of every range the resolver
// shifted onto the observed lines, and reports each correction: a rebase is
// never silent. Hashes stay as the model sent them; only the lines move.
func rebaseEdits(param EditInput, parsed []ParsedEdit, resolution editledger.Resolution) []string {
	if resolution.Delta == 0 || len(resolution.Lines) != len(parsed) {
		return nil
	}
	var notices []string
	for i, lines := range resolution.Lines {
		claimed := parsed[i].Spec
		if claimed.Start.Line == lines[0] && claimed.End.Line == lines[1] {
			continue
		}
		param.Edits[i].From = fmt.Sprintf("%d#%s", lines[0], claimed.Start.Hash)
		param.Edits[i].To = fmt.Sprintf("%d#%s", lines[1], claimed.End.Hash)
		notices = append(notices, fmt.Sprintf(
			"rebased edits[%d] from %d-%d to %d-%d (delta %+d)",
			i, claimed.Start.Line, claimed.End.Line, lines[0], lines[1], resolution.Delta,
		))
	}
	return notices
}

// refusalForOutcome renders a refused ledger claim. One typed outcome maps to
// exactly one What and one Next; the codes match doc/edit-capability.md.
func refusalForOutcome(outcome editledger.Outcome, display, tag string) error {
	switch outcome {
	case editledger.SnapshotConsumed:
		return &EditRefusal{
			Code: "snapshot_consumed",
			What: fmt.Sprintf(
				"the editable read of %s (TAG %s) was consumed by an edit that already applied",
				display,
				tag,
			),
			Next: "read it again with mode:\"edit\" and retry with the fresh TAG and LINE#HASH anchors",
		}
	case editledger.SnapshotEvicted:
		return &EditRefusal{
			Code: "snapshot_evicted",
			What: fmt.Sprintf(
				"the editable read of %s (TAG %s) fell out of the session ledger: too many files were read since",
				display,
				tag,
			),
			Next: "read it again with mode:\"edit\" and retry with the returned TAG and LINE#HASH anchors",
		}
	case editledger.SnapshotSuperseded:
		return &EditRefusal{
			Code: "snapshot_superseded",
			What: fmt.Sprintf(
				"the anchors of %s (TAG %s) predate a write of the file that already applied",
				display,
				tag,
			),
			Next: "use the TAG and LINE#HASH anchors printed by that write result, or read it again with mode:\"edit\"",
		}
	case editledger.AnchorNotObserved:
		return &EditRefusal{
			Code: "anchor_not_observed",
			What: fmt.Sprintf("an edits[] anchor was not part of the editable read of %s (TAG %s)", display, tag),
			Next: "retry with anchors copied exactly from that read, or read it again with mode:\"edit\"",
		}
	case editledger.MixedGrants:
		return &EditRefusal{
			Code: "mixed_grants",
			What: fmt.Sprintf(
				"a range's two anchors come from two different reads of %s; a single read must cover both endpoints",
				display,
			),
			Next: "read it once with mode:\"edit\" over the whole range and use only anchors from that read",
		}
	case editledger.AmbiguousReanchor:
		return &EditRefusal{
			Code: "ambiguous_reanchor",
			What: fmt.Sprintf(
				"a shifted edits[] anchor of %s (TAG %s) matches multiple candidate lines, or the anchors' shifts disagree",
				display,
				tag,
			),
			Next: "read it again with mode:\"edit\" and retry with the fresh LINE#HASH anchors",
		}
	case editledger.InvalidRef:
		return &EditRefusal{
			Code: "invalid_ref",
			What: `an edits[] reference is malformed: expected LINE#HASH anchors (e.g. "5#abc")`,
			Next: "retry with from/to copied exactly from the read output",
		}
	default:
		return &EditRefusal{
			Code: "no_capability",
			What: fmt.Sprintf("no current-session editable read of %s with TAG %s authorizes this edit", display, tag),
			Next: `read it with mode:"edit" (or grep with editable anchors), then retry with exactly the returned TAG and LINE#HASH anchors`,
		}
	}
}

// successorGrant is the live capability an applied edit mints for the next
// one: the new revision's TAG plus LINE#HASH anchors for the changed region.
// It is committed to the ledger verbatim; the result prints a subset of the
// same anchors, so everything the model sees authorizes the next edit.
type successorGrant struct {
	tag     string
	anchors []string
	// spans are the changed regions in the new file, clamped to its bounds:
	// the display shows them before the context that surrounds them.
	spans  [][2]int
	capped bool // the grant hit maxGeneratedGrantAnchors
}

// runParsedEdit applies a parsed edit and owns the claim's lifecycle end to
// end: any failure — read, TAG check, application, guarded swap — returns the
// claim to the ledger, and only a landed swap settles it with a successor
// grant for the exact new revision. Callers never order these by hand.
func runParsedEdit(
	ctx context.Context,
	param EditInput,
	notices []string,
	ledger *editledger.Ledger,
	claim *editledger.Claim,
) (tooldef.Result, error) {
	applied := false
	defer func() {
		if !applied {
			// The file is as it was, so the read that authorized this attempt
			// still describes it: hand the authorization back and let the model
			// correct the call instead of re-reading the file.
			ledger.Release(claim)
		}
	}()
	// Refusing to follow a leaf symlink keeps a swapped link from feeding
	// foreign content into the TAG check, the mismatch report or the diff.
	content, err := atomicfile.ReadNoFollow(param.Path)
	if err != nil {
		return tooldef.Result{}, err
	}
	fileContent := util.NormalizeLF(string(content))

	display := tooldef.RelToCwd(ctx, param.Path)
	actual := util.RevisionOf(fileContent)
	actualTag := actual.Tag()
	expectedTag := normalizeFileTag(param.Hash)
	if expectedTag == "" {
		return tooldef.Result{}, &EditRefusal{
			Code: "invalid_ref",
			What: fmt.Sprintf(
				"edit requires hash: the 4 hex chars after # in the @file path#TAG header from read/grep (e.g. A1B2 from %s)",
				util.FormatFileHeader(display, actualTag),
			),
			Next: "copy the TAG from the read header into the hash argument and retry",
		}
	}
	if refusal := staleRevision(claim, actual, expectedTag, display); refusal != nil {
		return tooldef.Result{}, refusal
	}

	newContent, dropped, spans, err := ApplyHashlineEdit(ctx, fileContent, param)
	if err != nil {
		return tooldef.Result{}, err
	}

	// The swap is guarded and atomic: every writer that goes through
	// atomicfile is serialized on this path, so one that touched the file
	// between the read above and the rename fails the edit instead of being
	// clobbered, and a crash mid-write cannot truncate the file. An arbitrary
	// process writing in the two syscalls before the rename is still lost —
	// the atomicfile package doc states that contract.
	opts := atomicfile.Options{
		Verify: unchangedRevisionGuard(actual, display),
		Guard:  mutationGuard(ctx),
	}
	if err := atomicfile.WriteWith(param.Path, destinationMode(param.Path), []byte(newContent), opts); err != nil {
		return tooldef.Result{}, err
	}
	applied = true

	newRev := util.RevisionOf(newContent)
	newTag := newRev.Tag()
	// A successor without a ledger authorizes nothing, so the ledger-less
	// path prints no anchors: the printed grant is always a real one.
	var successor successorGrant
	if claim != nil {
		successor = successorGrantFor(spans, strings.Split(newContent, "\n"), newTag)
		ledger.Commit(claim, newRev, successor.anchors)
	}
	diff := util.GenerateFileDiff(param.Path, fileContent, newContent, 3)
	var body strings.Builder
	body.WriteString(util.FormatFileHeader(display, newTag) + "\n")
	for _, notice := range notices {
		body.WriteString(notice + "\n")
	}
	if dropped > 0 {
		// Silently dropping identical edits left the model blind to its own
		// double-send; the count makes the dedup observable.
		fmt.Fprintf(&body,
			"edit_duplicate_dropped: %d duplicate edit(s) (same range and content) were dropped before applying\n",
			dropped)
	}
	if successor.tag != "" {
		writeSuccessorBlock(&body, successor)
	} else {
		body.WriteString("Re-read this file before another edit; prior LINE#HASH anchors are invalid.\n")
	}
	body.WriteString("\n" + diff)

	// The model re-reads the header + notice; the transcript diff card wants
	// only the hunks — the title row already names the path.
	return tooldef.Result{
		Content: body.String(),
		Detail:  display,
		Output:  diff,
	}, nil
}

// staleRevision reports the refusal for a file that is no longer the revision
// the edit was planned against. With a claim the comparison is the full
// revision identity: the display TAG is 16 bits, so a foreign write that
// happens to keep the TAG must not pass as the read content. Without a ledger
// there is nothing but the quoted TAG to compare against.
func staleRevision(claim *editledger.Claim, actual util.Revision, expectedTag, display string) error {
	actualTag := actual.Tag()
	if claim == nil {
		if expectedTag == actualTag {
			return nil
		}
		return tagChangedRefusal(expectedTag, actualTag, display)
	}
	if claim.Revision() == actual {
		return nil
	}
	if expectedTag == actualTag {
		return &EditRefusal{
			Code: "tag_changed",
			What: fmt.Sprintf(
				"file changed since the editable read: %s still shows TAG %s but its content differs",
				display,
				actualTag,
			),
			Next: "read it again with mode:\"edit\" and reapply the edit onto the new content",
		}
	}
	return tagChangedRefusal(expectedTag, actualTag, display)
}

func tagChangedRefusal(expectedTag, actualTag, display string) error {
	return &EditRefusal{
		Code: "tag_changed",
		What: fmt.Sprintf(
			"file TAG mismatch: edit.hash=%s but current file is %s",
			expectedTag,
			util.FormatFileHeader(display, actualTag),
		),
		Next: "read it again with mode:\"edit\" and copy the 4 hex chars after # before retrying",
	}
}

// unchangedRevisionGuard rejects the final swap when the file on disk is no
// longer the revision the edit was planned against: a concurrent writer landed
// in the window between the read and the write, and its changes must survive.
// The comparison is the full revision, so a foreign write that keeps the
// display TAG is caught too.
func unchangedRevisionGuard(rev util.Revision, display string) func(current []byte) error {
	return func(current []byte) error {
		got := util.RevisionOf(util.NormalizeLF(string(current)))
		if got == rev {
			return nil
		}
		if got.Tag() == rev.Tag() {
			return &EditRefusal{
				Code: "changed_during_edit",
				What: fmt.Sprintf(
					"file changed during edit: %s still shows TAG %s but its content differs",
					display,
					rev.Tag(),
				),
				Next: "read it again with mode:\"edit\" and reapply the edit onto the new content",
			}
		}
		return &EditRefusal{
			Code: "changed_during_edit",
			What: fmt.Sprintf(
				"file changed during edit: %s was %s when the edit started and is %s now",
				display,
				rev.Tag(),
				got.Tag(),
			),
			Next: "read it again with mode:\"edit\" and reapply the edit onto the new content",
		}
	}
}

// replacement is one applied edit's footprint in the original file: what it
// replaced and how long the replacement is.
type replacement struct {
	start, srcCount, dstLen int
}

// ApplyHashlineEdit applies flat hashline edits to fileContent and reports
// how many duplicate edits (same range and content) were dropped first, plus
// the (from, to) span each applied edit occupies in the NEW content — the
// successor grant is minted from those spans.
func ApplyHashlineEdit(ctx context.Context, fileContent string, param EditInput) (string, int, [][2]int, error) {
	lines := strings.Split(fileContent, "\n")
	parsed := make([]ParsedEdit, len(param.Edits))
	for i, fe := range param.Edits {
		var err error
		parsed[i], err = fe.toParsedEdit()
		if err != nil {
			return "", 0, nil, &EditRefusal{
				Code: "invalid_ref",
				What: fmt.Sprintf("edits[%d]: %s", i, err),
				Next: `retry with from/to as LINE#HASH anchors exactly as the read returned (e.g. "5#abc")`,
			}
		}
	}

	if err := validateLineReferences(parsed, lines); err != nil {
		return "", 0, nil, err
	}
	parsed = deduplicateParsedEdits(parsed)
	dropped := len(param.Edits) - len(parsed)

	annotated := getAnnotated(parsed)
	sort.Sort(bySortLine(annotated))

	// Each replacement shifts every later range, so overlapping edits would
	// splice into shifted offsets (or panic on nested ranges). Reject them
	// up front, after dedup, before anything is applied. annotated is
	// sorted bottom-up, so a range is safe when it ends above its neighbor.
	for i := 1; i < len(annotated); i++ {
		prev, cur := annotated[i-1].edit, annotated[i].edit
		if cur.Spec.End.Line >= prev.Spec.Start.Line {
			return "", 0, nil, &EditRefusal{
				Code: "overlap",
				What: fmt.Sprintf(
					"edits overlap: range %d-%d and range %d-%d share lines",
					prev.Spec.Start.Line,
					prev.Spec.End.Line,
					cur.Spec.Start.Line,
					cur.Spec.End.Line,
				),
				Next: "split the call into one edit per range and re-read the file between edits",
			}
		}
	}

	applied := make([]replacement, 0, len(annotated))
	for _, anno := range annotated {
		if ctx.Err() != nil {
			return "", 0, nil, ctx.Err()
		}
		edit := anno.edit
		count := edit.Spec.End.Line - edit.Spec.Start.Line + 1
		start := edit.Spec.Start.Line - 1
		lines = slices.Replace(lines, start, start+count, edit.Dst...)
		applied = append(applied, replacement{edit.Spec.Start.Line, count, len(edit.Dst)})
	}
	// A replacement only shifts what sits BELOW it in the file. The loop
	// above applied bottom-up, so each edit's final coordinates are its old
	// start plus the accumulated length deltas of the edits above it in the
	// file — which is this ascending pass.
	sort.Slice(applied, func(i, j int) bool { return applied[i].start < applied[j].start })
	spans := make([][2]int, 0, len(applied))
	shift := 0
	for _, rep := range applied {
		from := rep.start + shift
		spans = append(spans, [2]int{from, from + rep.dstLen - 1})
		shift += rep.dstLen - rep.srcCount
	}
	return strings.Join(lines, "\n"), dropped, spans, nil
}

// ---- Successor capability ----

const (
	// successorContextLines widens each changed span into a grant window: the
	// next edit usually targets the changed region or its immediate context.
	successorContextLines = 25
	// maxGeneratedGrantAnchors bounds the grant an edit mints; the union of
	// windows is truncated from the top, mirroring ObserveWrite.
	maxGeneratedGrantAnchors = 512
	// maxDisplayedAnchors bounds what the edit result prints.
	maxDisplayedAnchors = 40
)

// successorGrantFor expands the applied spans into the successor capability:
// each span plus context, merged, truncated to the grant cap from the top.
// A deleted range arrives as (s, s-1); the context window around it is the
// lines that survived next to the gap.
func successorGrantFor(spans [][2]int, newLines []string, newTag string) successorGrant {
	if len(spans) == 0 || newTag == "" {
		return successorGrant{}
	}
	total := len(newLines)
	windows := make([][2]int, 0, len(spans))
	changed := make([][2]int, 0, len(spans))
	for _, sp := range spans {
		from := max(1, sp[0]-successorContextLines)
		to := min(total, sp[1]+successorContextLines)
		if to >= from {
			windows = append(windows, [2]int{from, to})
		}
		if region, ok := changedRegion(sp, total); ok {
			changed = append(changed, region)
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i][0] < changed[j][0] })
	sort.Slice(windows, func(i, j int) bool { return windows[i][0] < windows[j][0] })
	merged := windows[:0]
	for _, w := range windows {
		if n := len(merged); n > 0 && w[0] <= merged[n-1][1]+1 {
			merged[n-1][1] = max(merged[n-1][1], w[1])
			continue
		}
		merged = append(merged, w)
	}
	grant := successorGrant{tag: newTag, spans: changed}
	for _, w := range merged {
		for line := w[0]; line <= w[1] && len(grant.anchors) < maxGeneratedGrantAnchors; line++ {
			grant.anchors = append(grant.anchors, fmt.Sprintf("%d#%s", line, util.ComputeLineHash(newLines[line-1])))
		}
		if len(grant.anchors) >= maxGeneratedGrantAnchors {
			// The union ran past the cap: what stayed outside the grant must be
			// re-read, and the model has to know that.
			grant.capped = true
			break
		}
	}
	return grant
}

// changedRegion clamps an applied span to the new file's bounds. A deletion
// arrives as the empty gap (s, s-1): the lines that now sit on either side of
// the gap are what the next edit will aim at, so they are its changed region.
func changedRegion(span [2]int, total int) ([2]int, bool) {
	lo, hi := span[0], span[1]
	if hi < lo {
		lo, hi = span[0]-1, span[0]
	}
	lo = max(1, lo)
	hi = min(total, hi)
	if hi < lo {
		return [2]int{}, false
	}
	return [2]int{lo, hi}, true
}

// displayedAnchors chooses the anchors the result prints. The grant is up to
// maxGeneratedGrantAnchors lines wide but the result shows a fraction of it,
// so the choice decides what the model can act on without a re-read: every
// changed line first — spread across the changed regions so no edited region
// is invisible, each region's endpoints before its interior — and only then
// context, expanding outward from each region a line at a time, nearest
// first. The result is a subset of grant.anchors in ascending line order, so
// everything printed is authorized.
func displayedAnchors(grant successorGrant, budget int) []string {
	if budget <= 0 {
		return nil
	}
	if len(grant.anchors) <= budget {
		return grant.anchors
	}
	byLine := make(map[int]int, len(grant.anchors))
	for i, anchor := range grant.anchors {
		byLine[anchorLine(anchor)] = i
	}
	picked := make([]bool, len(grant.anchors))
	count := 0
	// take claims one granted line; it reports false when the line is outside
	// the grant, which is also how a context walk learns its window ended.
	take := func(line int) bool {
		i, ok := byLine[line]
		if !ok {
			return false
		}
		if !picked[i] {
			picked[i] = true
			count++
		}
		return true
	}

	queues := changedLineQueues(grant, byLine)
	for round := 0; count < budget; round++ {
		advanced := false
		for _, queue := range queues {
			if round >= len(queue) {
				continue
			}
			advanced = true
			take(queue[round])
			if count >= budget {
				break
			}
		}
		if !advanced {
			break
		}
	}

	// Context: one cursor per region and direction, stepped round-robin, so
	// the budget left over spreads evenly instead of draining into the first
	// region's window.
	cursors := make([]contextCursor, 0, 2*len(grant.spans))
	for _, sp := range grant.spans {
		cursors = append(cursors, contextCursor{line: sp[0] - 1, dir: -1}, contextCursor{line: sp[1] + 1, dir: 1})
	}
	for count < budget {
		advanced := false
		for i := range cursors {
			if count >= budget {
				break
			}
			if cursors[i].step(take) {
				advanced = true
			}
		}
		if !advanced {
			break
		}
	}

	shown := make([]string, 0, count)
	for i, anchor := range grant.anchors {
		if picked[i] {
			shown = append(shown, anchor)
		}
	}
	return shown
}

// changedLineQueues orders each region's granted lines for display: first
// line, last line, then the interior ascending — both ends of an edited
// region stay visible even when its middle does not fit.
func changedLineQueues(grant successorGrant, byLine map[int]int) [][]int {
	queues := make([][]int, 0, len(grant.spans))
	for _, sp := range grant.spans {
		present := make([]int, 0, min(sp[1]-sp[0]+1, len(byLine)))
		for line := sp[0]; line <= sp[1]; line++ {
			if _, ok := byLine[line]; ok {
				present = append(present, line)
			}
		}
		switch len(present) {
		case 0:
			continue
		case 1, 2:
			queues = append(queues, present)
		default:
			queue := append([]int{present[0], present[len(present)-1]}, present[1:len(present)-1]...)
			queues = append(queues, queue)
		}
	}
	return queues
}

// contextCursor walks away from a changed region one line at a time. It dies
// where the grant window does: a line the grant never covered cannot be
// displayed, and nothing beyond it in that direction can either.
type contextCursor struct {
	line int
	dir  int
	dead bool
}

// step walks this cursor one line further from its region and claims it. A
// line already taken as a changed line claims nothing but keeps the cursor
// alive: two regions sharing a window walk past each other instead of dying.
func (c *contextCursor) step(take func(int) bool) bool {
	if c.dead {
		return false
	}
	line := c.line
	c.line += c.dir
	if !take(line) {
		c.dead = true
		return false
	}
	return true
}

// writeSuccessorBlock renders the successor capability as the edit result's
// live anchors: the message says plainly that these authorize the next edit
// and that every prior anchor died with the old revision.
func writeSuccessorBlock(body *strings.Builder, grant successorGrant) {
	shown := displayedAnchors(grant, maxDisplayedAnchors)
	fmt.Fprintf(
		body,
		"These LINE#HASH anchors are live and authorize the next edit of the changed region with hash=%s; all prior anchors are invalid:\n",
		grant.tag,
	)
	prev := 0
	for i, anchor := range shown {
		line := anchorLine(anchor)
		switch {
		case i == 0:
		case line == prev+1:
			body.WriteByte(' ')
		default:
			// A jump in the printed anchors is not a jump in the file: mark it,
			// or the model reads two distant lines as neighbors.
			body.WriteString(" … ")
		}
		body.WriteString(anchor)
		prev = line
	}
	body.WriteByte('\n')
	if rest := len(grant.anchors) - len(shown); rest > 0 {
		fmt.Fprintf(body,
			"+%d more live anchors not shown (lines %s); read those ranges with "+
				"mode:\"edit\" (offset/limit) or use the anchors above\n",
			rest, formatLineRanges(omittedRanges(grant.anchors, shown)))
	}
	if grant.capped {
		fmt.Fprintf(body,
			"the grant covers the first %d anchor lines of the changed region; beyond them read with mode:\"edit\"\n",
			maxGeneratedGrantAnchors)
	}
}

// omittedRanges merges the granted lines the display left out into ranges, so
// the message can name exactly what to read instead of only how much is
// missing. shown is a subset of all in the same order.
func omittedRanges(all, shown []string) [][2]int {
	ranges := make([][2]int, 0, len(shown)+1)
	next := 0
	for _, anchor := range all {
		if next < len(shown) && shown[next] == anchor {
			next++
			continue
		}
		line := anchorLine(anchor)
		if n := len(ranges); n > 0 && ranges[n-1][1]+1 == line {
			ranges[n-1][1] = line
			continue
		}
		ranges = append(ranges, [2]int{line, line})
	}
	return ranges
}

func formatLineRanges(ranges [][2]int) string {
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r[0] == r[1] {
			parts = append(parts, strconv.Itoa(r[0]))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
	}
	return strings.Join(parts, ", ")
}

// anchorLine reports the LINE of a "LINE#HASH" anchor. The grant mints its
// own anchors, so a malformed one is impossible; 0 keeps the printer honest
// rather than panicking if that ever stops being true.
func anchorLine(anchor string) int {
	i := strings.IndexByte(anchor, '#')
	if i <= 0 {
		return 0
	}
	line, err := strconv.Atoi(anchor[:i])
	if err != nil {
		return 0
	}
	return line
}

// ---- Parsing ----

func parseEditInput(ctx context.Context, raw json.RawMessage) (EditInput, error) {
	var param EditInput
	if err := json.Unmarshal(raw, &param); err != nil {
		return EditInput{}, fmt.Errorf("failed to parse edit arguments: %w", err)
	}
	param.Path = strings.TrimSpace(param.Path)
	if param.Path == "" {
		param.Path = strings.TrimSpace(tooldef.FilePathAlias(raw))
	}
	if param.Path == "" {
		return EditInput{}, errors.New("edit requires a non-empty path: provide the same path you passed to read")
	}
	abs, err := tooldef.ResolveToCwd(ctx, param.Path)
	if err != nil {
		return EditInput{}, err
	}
	param.Path = abs
	return param, nil
}

// normalizeFileTag extracts the 4-hex TAG from common copy-paste forms:
// "A1B2", "#A1B2", "@file src/app.py#A1B2".
func normalizeFileTag(hash string) string {
	s := strings.TrimSpace(hash)
	if i := strings.LastIndex(s, "#"); i >= 0 {
		s = s[i+1:]
	}
	return strings.ToUpper(strings.TrimSpace(s))
}

func (f FlatEdit) toParsedEdit() (ParsedEdit, error) {
	from := strings.TrimSpace(f.From)
	to := strings.TrimSpace(f.To)
	if from == "" || to == "" {
		return ParsedEdit{}, errors.New("edit requires non-empty from and to (LINE#HASH each)")
	}
	sl, sh, err := parseLineRef(from)
	if err != nil {
		return ParsedEdit{}, err
	}
	el, eh, err := parseLineRef(to)
	if err != nil {
		return ParsedEdit{}, err
	}
	lines := contentLines(f.Content)
	if lines == nil {
		lines = []string{}
	}
	return ParsedEdit{
		Spec: ParsedRef{
			Start: LineRef{Line: sl, Hash: sh},
			End:   LineRef{Line: el, Hash: eh},
		},
		Dst: lines,
	}, nil
}

func contentLines(content *string) []string {
	if content == nil {
		return nil
	}
	s := strings.ReplaceAll(*content, "\r", "")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{}
	}
	return util.StripLinePrefixes(strings.Split(s, "\n"))
}

// ---- Line reference parsing ----

var hashLen = util.LineHashLen

// lineRefPattern parses "5#abc", "  5  #  abc", "> 5#abc|content", etc.
var lineRefPattern = regexp.MustCompile(fmt.Sprintf(`^\s*[>+-]*\s*(\d+)\s*[:#]\s*([a-zA-Z]{%d})`, hashLen))

func parseLineRef(ref string) (int, string, error) {
	if strings.ContainsAny(ref, "\n\r") {
		return 0, "", errors.New(`from/to must be a single LINE#HASH (e.g. "5#abc"), not a pasted block`)
	}
	match := lineRefPattern.FindStringSubmatch(ref)
	if match == nil {
		return 0, "", fmt.Errorf(
			`invalid line reference %q. Expected format "LINE#HASH" (e.g. "5#abc")`,
			ref,
		)
	}
	line, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, "", fmt.Errorf("invalid line number in reference %q: %w", ref, err)
	}
	if line < 1 {
		return 0, "", fmt.Errorf("line number must be >= 1, got %d in %q", line, ref)
	}
	return line, match[2], nil
}

// ---- Sorting ----

type bySortLine []Annotated

func (a bySortLine) Len() int      { return len(a) }
func (a bySortLine) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a bySortLine) Less(i, j int) bool {
	if a[i].sortLine != a[j].sortLine {
		return a[i].sortLine > a[j].sortLine
	}
	return a[i].index < a[j].index
}

func getAnnotated(parsed []ParsedEdit) []Annotated {
	annotated := make([]Annotated, 0, len(parsed))
	for i, p := range parsed {
		annotated = append(annotated, Annotated{
			edit:     p,
			index:    i,
			sortLine: p.Spec.End.Line,
		})
	}
	return annotated
}

// ---- Validation ----

func validateLineReferences(parsed []ParsedEdit, contents []string) error {
	l := len(contents)
	var mismatches []HashMismatch

	for _, p := range parsed {
		if p.Spec.Start.Line > p.Spec.End.Line {
			return &EditRefusal{
				Code: "range_inverted",
				What: fmt.Sprintf("range start line %d must be <= end line %d", p.Spec.Start.Line, p.Spec.End.Line),
				Next: "put the two anchors in file order and retry",
			}
		}
		if p.Spec.Start.Line < 1 || p.Spec.End.Line > l {
			return &EditRefusal{
				Code: "out_of_bounds",
				What: fmt.Sprintf(
					"line range %d-%d is out of bounds (file has %d lines)",
					p.Spec.Start.Line,
					p.Spec.End.Line,
					l,
				),
				Next: "read the file again with mode:\"edit\" to get valid anchors",
			}
		}
		if !util.ValidateHash(p.Spec.Start.Line, p.Spec.Start.Hash, contents) {
			mismatches = append(mismatches, hashMismatch(contents, p.Spec.Start.Line, p.Spec.Start.Hash))
		}
		if !util.ValidateHash(p.Spec.End.Line, p.Spec.End.Hash, contents) {
			mismatches = append(mismatches, hashMismatch(contents, p.Spec.End.Line, p.Spec.End.Hash))
		}
	}

	if len(mismatches) > 0 {
		return newHashlineMismatchError(mismatches, contents)
	}
	return nil
}

func hashMismatch(contents []string, line int, expectedHash string) HashMismatch {
	actual := ""
	if line >= 1 && line <= len(contents) {
		actual = util.ComputeLineHash(contents[line-1])
	}
	return HashMismatch{Line: line, Expected: expectedHash, Actual: actual}
}

// ---- Dedup ----

func deduplicateParsedEdits(parsed []ParsedEdit) []ParsedEdit {
	if len(parsed) <= 1 {
		return parsed
	}
	type key struct {
		loc string
		dst string
	}
	seen := make(map[key]int, len(parsed))
	dedupe := make(map[int]struct{})

	for i, p := range parsed {
		k := key{
			loc: fmt.Sprintf("r:%d:%d", p.Spec.Start.Line, p.Spec.End.Line),
			dst: strings.Join(p.Dst, "\n"),
		}
		if _, ok := seen[k]; ok {
			dedupe[i] = struct{}{}
		} else {
			seen[k] = i
		}
	}

	if len(dedupe) == 0 {
		return parsed
	}

	filtered := make([]ParsedEdit, 0, len(parsed)-len(dedupe))
	for i, p := range parsed {
		if _, drop := dedupe[i]; drop {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered
}

// ---- Mismatch error formatting ----

func newHashlineMismatchError(mismatches []HashMismatch, fileLines []string) *HashlineMismatchError {
	const contextLines = 2

	displaySet := make(map[int]struct{})
	for _, m := range mismatches {
		for i := m.Line - contextLines; i <= m.Line+contextLines; i++ {
			if i < 1 || i > len(fileLines) {
				continue
			}
			displaySet[i] = struct{}{}
		}
	}

	displayLines := make([]int, 0, len(displaySet))
	for ln := range displaySet {
		displayLines = append(displayLines, ln)
	}
	sort.Ints(displayLines)

	var b strings.Builder
	plural := ""
	if len(mismatches) > 1 {
		plural = "s"
	}
	fmt.Fprintf(
		&b,
		"[edit:tag_changed] %d line%s have changed since last read. Do not retry the same call unchanged. Use the updated LINE#HASH references shown below (>>> marks changed lines).",
		len(mismatches),
		plural,
	)
	b.WriteString("\n\n")

	mismatchByLine := make(map[int]HashMismatch, len(mismatches))
	for _, m := range mismatches {
		mismatchByLine[m.Line] = m
	}

	prev := -1
	for _, ln := range displayLines {
		if prev != -1 && ln > prev+1 {
			b.WriteString("    ...\n")
		}
		prev = ln

		text := ""
		if ln-1 >= 0 && ln-1 < len(fileLines) {
			text = fileLines[ln-1]
		}
		hash := util.ComputeLineHash(text)
		prefix := fmt.Sprintf("%d#%s", ln, hash)

		if _, ok := mismatchByLine[ln]; ok {
			fmt.Fprintf(&b, ">>> %s|%s\n", prefix, text)
		} else {
			fmt.Fprintf(&b, "    %s|%s\n", prefix, text)
		}
	}

	return &HashlineMismatchError{
		mismatches: mismatches,
		fileLines:  fileLines,
		msg:        b.String(),
	}
}

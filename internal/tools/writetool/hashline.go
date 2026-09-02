package writetool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

var editDescription = `Edit a file using a whole-file TAG and LINE#HASH anchors from a current-session
read with mode:"edit" (or editable grep output). View reads do not authorize edits.

Required hash: the 4 hex chars AFTER # in the @file path#TAG header
(e.g. A1B2 from "@file src/app.py#A1B2") — not "@file", not the path, not the #.
Put multiple changes to the same file in one edits array — they share one TAG
and apply against the same original snapshot. A successful edit ends the
authorization — re-read before editing that file again; a failed one keeps it,
so fix the call and retry without re-reading.

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

// ---- Main entry points ----

func runEdit(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	param, err := parseEditInput(ctx, input)
	if err != nil {
		return tooldef.Result{}, err
	}
	return runParsedEdit(ctx, param)
}

func runAuthorizedEdit(ctx context.Context, input json.RawMessage, ledger *editledger.Ledger) (tooldef.Result, error) {
	param, err := parseEditInput(ctx, input)
	if err != nil {
		return tooldef.Result{}, err
	}
	refs := make([]string, 0, len(param.Edits)*2)
	for _, edit := range param.Edits {
		refs = append(refs, edit.From, edit.To)
	}
	claim, ok := ledger.Claim(param.Path, normalizeFileTag(param.Hash), refs)
	if !ok {
		return tooldef.Result{}, errors.New(
			`edit is not authorized by a current-session editable read; use read with mode:"edit" and retry with exactly the returned TAG and LINE#HASH anchors`,
		)
	}
	result, err := runParsedEdit(ctx, param)
	if err != nil {
		// The file is as it was, so the read that authorized this attempt still
		// describes it: hand the authorization back and let the model correct
		// the call instead of re-reading the file.
		ledger.Release(claim)
		return tooldef.Result{}, err
	}
	return result, nil
}

func runParsedEdit(ctx context.Context, param EditInput) (tooldef.Result, error) {
	// Refusing to follow a leaf symlink keeps a swapped link from feeding
	// foreign content into the TAG check, the mismatch report or the diff.
	content, err := atomicfile.ReadNoFollow(param.Path)
	if err != nil {
		return tooldef.Result{}, err
	}
	fileContent := util.NormalizeLF(string(content))

	display := tooldef.RelToCwd(ctx, param.Path)
	actualTag := util.ComputeFileHash(fileContent)
	expectedTag := normalizeFileTag(param.Hash)
	if expectedTag == "" {
		return tooldef.Result{}, fmt.Errorf(
			"edit requires hash: the 4 hex chars after # in the @file path#TAG header from read/grep (e.g. A1B2 from %s)",
			util.FormatFileHeader(display, actualTag),
		)
	}
	if expectedTag != actualTag {
		return tooldef.Result{}, fmt.Errorf(
			"file TAG mismatch: edit.hash=%s but current file is %s. Re-read the file and copy the 4 hex chars after # before retrying",
			expectedTag,
			util.FormatFileHeader(display, actualTag),
		)
	}

	newContent, err := ApplyHashlineEdit(ctx, fileContent, param)
	if err != nil {
		return tooldef.Result{}, err
	}

	// The swap is guarded and atomic: a concurrent writer that touched the
	// file between the read above and the rename fails the edit instead of
	// being clobbered, and a crash mid-write cannot truncate the file.
	mode := os.FileMode(0o644)
	// Lstat reads the path itself, not what a swapped link points at; a leaf
	// symlink is refused inside the mutation module anyway.
	if info, err := os.Lstat(param.Path); err == nil {
		mode = info.Mode().Perm() // an edit rewrites content, not permissions
	}
	guard := unchangedTagGuard(expectedTag, display)
	if err := atomicfile.WriteChecked(param.Path, mode, []byte(newContent), guard); err != nil {
		return tooldef.Result{}, err
	}

	newTag := util.ComputeFileHash(newContent)
	diff := util.GenerateFileDiff(param.Path, fileContent, newContent, 3)
	body := util.FormatFileHeader(display, newTag) +
		"\nRe-read this file before another edit; prior LINE#HASH anchors are invalid.\n\n" +
		diff

	// The model re-reads the header + notice; the transcript diff card wants
	// only the hunks — the title row already names the path.
	return tooldef.Result{
		Content: body,
		Detail:  display,
		Output:  diff,
	}, nil
}

// unchangedTagGuard rejects the final swap when the file on disk no longer
// hashes to the tag the edit was planned against: a concurrent writer landed
// in the window between the read and the write, and its changes must survive.
func unchangedTagGuard(tag, display string) func(current []byte) error {
	return func(current []byte) error {
		got := util.ComputeFileHash(util.NormalizeLF(string(current)))
		if got == tag {
			return nil
		}
		return fmt.Errorf(
			"file changed during edit: %s was %s when the edit started and is %s now. "+
				"Re-read the file and copy the 4 hex chars after # before retrying",
			display,
			tag,
			got,
		)
	}
}

// ApplyHashlineEdit applies flat hashline edits to fileContent.
func ApplyHashlineEdit(ctx context.Context, fileContent string, param EditInput) (string, error) {
	lines := strings.Split(fileContent, "\n")
	parsed := make([]ParsedEdit, len(param.Edits))
	for i, fe := range param.Edits {
		var err error
		parsed[i], err = fe.toParsedEdit()
		if err != nil {
			return "", fmt.Errorf("edits[%d]: %w", i, err)
		}
	}

	if err := validateLineReferences(parsed, lines); err != nil {
		return "", err
	}
	parsed = deduplicateParsedEdits(parsed)

	annotated := getAnnotated(parsed)
	sort.Sort(bySortLine(annotated))

	for _, anno := range annotated {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		edit := anno.edit
		count := edit.Spec.End.Line - edit.Spec.Start.Line + 1
		start := edit.Spec.Start.Line - 1
		lines = slices.Replace(lines, start, start+count, edit.Dst...)
	}
	return strings.Join(lines, "\n"), nil
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
			return fmt.Errorf("range start line %d must be <= end line %d", p.Spec.Start.Line, p.Spec.End.Line)
		}
		if p.Spec.Start.Line < 1 || p.Spec.End.Line > l {
			return fmt.Errorf(
				"line range %d-%d is out of bounds (file has %d lines). Re-read the file to get valid anchors",
				p.Spec.Start.Line,
				p.Spec.End.Line,
				l,
			)
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
		"%d line%s have changed since last read. Use the updated LINE#HASH references shown below (>>> marks changed lines).",
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

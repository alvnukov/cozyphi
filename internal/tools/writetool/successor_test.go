package writetool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/util"
)

// ---- Successor display selection ----

// A replacement in the middle of a long file must show every line it changed.
// The old display printed the grant's first anchors in ascending order, so the
// context above the edit crowded out the tail of the edit itself.
func TestEditResultShowsEveryChangedLine(t *testing.T) {
	const editStart, editEnd = 100, 119
	ledger := editledger.New()
	path, lines := authorizedFile(t, ledger, 200)
	replacement := numberedLines("new", editEnd-editStart+1)

	res, err := EditTool(ledger).Run(t.Context(), editArgs(t, path, lines, [][2]int{{editStart, editEnd}}, replacement))
	require.NoError(t, err)

	shown := shownAnchors(t, res.Content)
	require.LessOrEqual(t, len(shown), maxDisplayedAnchors)
	for i, line := range replacement {
		require.Contains(t, shown, hashlineRef(editStart+i, line),
			"changed line %d must be visible in the result", editStart+i)
	}
	requireAnchorsAuthorize(t, ledger, path, applied(lines, [][2]int{{editStart, editEnd}}, replacement), shown)
}

// Two edits far apart in the same call: the display budget is split between
// them, so neither region is invisible.
func TestEditResultShowsEveryChangedRegion(t *testing.T) {
	first, second := [2]int{10, 12}, [2]int{150, 152}
	ledger := editledger.New()
	path, lines := authorizedFile(t, ledger, 200)
	replacement := numberedLines("new", 3)

	res, err := EditTool(ledger).Run(t.Context(), editArgs(t, path, lines, [][2]int{first, second}, replacement))
	require.NoError(t, err)

	shown := shownAnchors(t, res.Content)
	require.LessOrEqual(t, len(shown), maxDisplayedAnchors)
	for _, region := range [][2]int{first, second} {
		for i, line := range replacement {
			require.Contains(t, shown, hashlineRef(region[0]+i, line),
				"changed line %d must be visible in the result", region[0]+i)
		}
	}
	requireAnchorsAuthorize(t, ledger, path, applied(lines, [][2]int{first, second}, replacement), shown)
}

// The omitted-ranges line names the exact windows the model cannot see, so a
// line it needs is one bounded read away instead of a whole-file re-read.
func TestOmittedRangesMessageNamesUnseenWindows(t *testing.T) {
	const editStart, editEnd = 100, 119
	ledger := editledger.New()
	path, lines := authorizedFile(t, ledger, 200)
	replacement := numberedLines("new", editEnd-editStart+1)

	res, err := EditTool(ledger).Run(t.Context(), editArgs(t, path, lines, [][2]int{{editStart, editEnd}}, replacement))
	require.NoError(t, err)

	// The changed lines are shown first; what is left of the budget expands
	// outward from the region, the upward step going first each round.
	changed := editEnd - editStart + 1
	context := maxDisplayedAnchors - changed
	above, below := (context+1)/2, context/2
	want := fmt.Sprintf(
		"+%d more live anchors not shown (lines %d-%d, %d-%d); "+
			"read those ranges with mode:\"edit\" (offset/limit) or use the anchors above",
		2*successorContextLines+changed-maxDisplayedAnchors,
		editStart-successorContextLines, editStart-above-1,
		editEnd+below+1, editEnd+successorContextLines,
	)
	require.Contains(t, res.Content, want)
}

// An edit whose window runs past the grant cap: the truncation message stays,
// and the omitted ranges account for every granted line the display dropped.
func TestCappedGrantNamesOmittedRanges(t *testing.T) {
	const total, editStart = 700, 100
	ledger := editledger.New()
	path, lines := authorizedFile(t, ledger, total)
	replacement := numberedLines("new", total-editStart+1)

	res, err := EditTool(ledger).Run(t.Context(), editArgs(t, path, lines, [][2]int{{editStart, total}}, replacement))
	require.NoError(t, err)
	require.Contains(t, res.Content, "beyond them read with mode")

	shown := shownLines(t, res.Content)
	require.Len(t, shown, maxDisplayedAnchors)
	omitted := omittedLines(t, res.Content)
	require.Len(t, omitted, maxGeneratedGrantAnchors-maxDisplayedAnchors)

	// Shown and omitted partition the grant: the message accounts for every
	// live anchor the model cannot see, and claims none it can.
	firstGranted := editStart - successorContextLines
	granted := make([]int, 0, maxGeneratedGrantAnchors)
	for line := firstGranted; line < firstGranted+maxGeneratedGrantAnchors; line++ {
		granted = append(granted, line)
	}
	require.Equal(t, granted, slices.Sorted(slices.Values(slices.Concat(shown, omitted))))

	// The display reaches the far end of the grant, and the gap in between is
	// marked rather than left to read as adjacency.
	require.Contains(t, res.Content, " … ")
	require.Equal(t, granted[len(granted)-1], shown[len(shown)-1])
}

// The pure selection: a subset of the grant, ascending, changed lines before
// context, and the leftover budget shared between the changed regions.
func TestDisplayedAnchorsSelectsChangedLinesFirst(t *testing.T) {
	lines := numberedLines("old", 300)
	regions := [][2]int{{40, 44}, {200, 204}}
	grant := successorGrantFor(regions, lines, "AB12")
	require.Greater(t, len(grant.anchors), maxDisplayedAnchors)

	shown := displayedAnchors(grant, maxDisplayedAnchors)
	require.Len(t, shown, maxDisplayedAnchors)
	require.Subset(t, grant.anchors, shown)
	require.True(t, slices.IsSorted(anchorLines(shown)), "displayed anchors print in file order")

	for _, region := range regions {
		for line := region[0]; line <= region[1]; line++ {
			require.Contains(t, shown, fmt.Sprintf("%d#%s", line, util.ComputeLineHash(lines[line-1])))
		}
		// Neither region is starved: the context around each one is displayed.
		require.Contains(t, shown, fmt.Sprintf("%d#%s", region[0]-1, util.ComputeLineHash(lines[region[0]-2])))
		require.Contains(t, shown, fmt.Sprintf("%d#%s", region[1]+1, util.ComputeLineHash(lines[region[1]])))
	}
}

// A grant that fits in the budget is printed whole.
func TestDisplayedAnchorsShowsSmallGrantWhole(t *testing.T) {
	lines := numberedLines("old", 10)
	grant := successorGrantFor([][2]int{{4, 5}}, lines, "AB12")
	require.Equal(t, grant.anchors, displayedAnchors(grant, maxDisplayedAnchors))
}

// A deletion leaves no changed line of its own; its region is the pair of
// lines the gap now sits between, so the display still shows where it landed.
func TestSuccessorGrantSpansCoverDeletionNeighbours(t *testing.T) {
	lines := numberedLines("old", 4)
	grant := successorGrantFor([][2]int{{2, 1}}, lines[:3], "AB12")
	require.Equal(t, [][2]int{{1, 2}}, grant.spans)
}

// ---- Helpers ----

func numberedLines(prefix string, n int) []string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s-%04d", prefix, i+1)
	}
	return lines
}

// authorizedFile writes an n-line file and grants the ledger a capability over
// all of it, the way read(mode:"edit") does.
func authorizedFile(t *testing.T, ledger *editledger.Ledger, n int) (string, []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "long.txt")
	lines := numberedLines("old", n)
	content := strings.Join(lines, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	anchors := make([]string, 0, len(lines))
	for i, line := range lines {
		anchors = append(anchors, hashlineRef(i+1, line))
	}
	ledger.Authorize(path, util.RevisionOf(content), anchors)
	return path, lines
}

// editArgs builds an edit call replacing each region with the same lines.
func editArgs(t *testing.T, path string, lines []string, regions [][2]int, replacement []string) json.RawMessage {
	t.Helper()
	edits := make([]FlatEdit, 0, len(regions))
	for _, region := range regions {
		edits = append(edits, FlatEdit{
			From:    hashlineRef(region[0], lines[region[0]-1]),
			To:      hashlineRef(region[1], lines[region[1]-1]),
			Content: new(strings.Join(replacement, "\n")),
		})
	}
	raw, err := json.Marshal(EditInput{Path: path, Hash: util.ComputeFileHash(strings.Join(lines, "\n")), Edits: edits})
	require.NoError(t, err)
	return raw
}

// applied reports the TAG of the file those edits produce.
func applied(lines []string, regions [][2]int, replacement []string) string {
	out := slices.Clone(lines)
	for _, region := range slices.Backward(regions) {
		out = slices.Replace(out, region[0]-1, region[1], replacement...)
	}
	return util.ComputeFileHash(strings.Join(out, "\n"))
}

// shownAnchors returns the anchors the result printed, in print order. The gap
// marker separates anchors; it is not one.
func shownAnchors(t *testing.T, content string) []string {
	t.Helper()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "These LINE#HASH anchors are live") {
			continue
		}
		require.Greater(t, len(lines), i+1, "the anchor list follows its header")
		anchors := make([]string, 0, maxDisplayedAnchors)
		for field := range strings.FieldsSeq(lines[i+1]) {
			if field != "…" {
				anchors = append(anchors, field)
			}
		}
		return anchors
	}
	t.Fatalf("the result printed no successor anchors:\n%s", content)
	return nil
}

func shownLines(t *testing.T, content string) []int {
	t.Helper()
	return anchorLines(shownAnchors(t, content))
}

func anchorLines(anchors []string) []int {
	lines := make([]int, 0, len(anchors))
	for _, anchor := range anchors {
		lines = append(lines, anchorLine(anchor))
	}
	return lines
}

// omittedLines expands the "(lines A-B, C)" list of the not-shown message.
func omittedLines(t *testing.T, content string) []int {
	t.Helper()
	const marker = "more live anchors not shown (lines "
	_, rest, found := strings.Cut(content, marker)
	require.True(t, found, "the result named no omitted ranges:\n%s", content)
	list, _, found := strings.Cut(rest, ")")
	require.True(t, found)

	lines := make([]int, 0, maxGeneratedGrantAnchors)
	for part := range strings.SplitSeq(list, ", ") {
		from, to, isRange := strings.Cut(part, "-")
		if !isRange {
			to = from
		}
		start, err := strconv.Atoi(from)
		require.NoError(t, err)
		end, err := strconv.Atoi(to)
		require.NoError(t, err)
		for line := start; line <= end; line++ {
			lines = append(lines, line)
		}
	}
	return lines
}

// requireAnchorsAuthorize claims every printed anchor: a displayed anchor that
// authorizes nothing is exactly what this display must never print.
func requireAnchorsAuthorize(t *testing.T, ledger *editledger.Ledger, path, tag string, anchors []string) {
	t.Helper()
	require.NotEmpty(t, anchors)
	for _, anchor := range anchors {
		line, hash, err := parseLineRef(anchor)
		require.NoError(t, err)
		ref := editledger.Ref{Line: line, Hash: hash}
		claim, resolution := ledger.Claim(path, tag, []editledger.Ref{ref, ref})
		require.False(t, resolution.Outcome.Refused(),
			"displayed anchor %s must authorize the next edit, got %s", anchor, resolution.Outcome.Code())
		ledger.Release(claim)
	}
}

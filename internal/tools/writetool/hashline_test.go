package writetool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestApplyHashlineEdit(t *testing.T) {
	betaHash := util.ComputeLineHash("beta")
	gammaHash := util.ComputeLineHash("gamma")
	staleBetaHash := differentHash(betaHash)
	staleGammaHash := differentHash(gammaHash)

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name         string
		ctx          context.Context // nil means t.Context()
		fileContent  string
		edits        []FlatEdit
		want         string
		wantErrIs    error
		wantMismatch *HashMismatch // expect HashlineMismatchError with exactly this mismatch
		wantErr      string        // expect error message containing this
	}{
		{
			name:        "replaces a single line",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			want: "alpha\nBETA\ngamma",
		},
		{
			name:        "replaces an inclusive range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(3, "gamma"),
				Content: new("combined"),
			}},
			want: "alpha\ncombined",
		},
		{
			name:        "deletes a range when content is nil",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From: hashlineRef(2, "beta"),
				To:   hashlineRef(3, "gamma"),
			}},
			want: "alpha",
		},
		{
			name:        "uses original anchors for multiple edits",
			fileContent: "one\ntwo\nthree\nfour\nfive",
			edits: []FlatEdit{
				{
					From:    hashlineRef(2, "two"),
					To:      hashlineRef(2, "two"),
					Content: new("two-a\ntwo-b\ntwo-c"),
				},
				{
					From: hashlineRef(5, "five"),
					To:   hashlineRef(5, "five"),
				},
			},
			want: "one\ntwo-a\ntwo-b\ntwo-c\nthree\nfour",
		},
		{
			name:        "deduplicates identical edits",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{
				{
					From:    hashlineRef(2, "beta"),
					To:      hashlineRef(2, "beta"),
					Content: new("beta-a\nbeta-b"),
				},
				{
					From:    hashlineRef(2, "beta"),
					To:      hashlineRef(2, "beta"),
					Content: new("beta-a\nbeta-b"),
				},
			},
			want: "alpha\nbeta-a\nbeta-b\ngamma",
		},
		{
			name:        "rejects a stale hash",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    fmt.Sprintf("2#%s", staleBetaHash),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			wantMismatch: &HashMismatch{Line: 2, Expected: staleBetaHash, Actual: betaHash},
		},
		{
			name:        "rejects a reversed range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(3, "gamma"),
				To:      hashlineRef(2, "beta"),
				Content: new("unused"),
			}},
			wantErr: "range start line 3 must be <= end line 2",
		},
		{
			name:        "rejects an out-of-bounds range",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(1, "alpha"),
				To:      "4#aaa",
				Content: new("unused"),
			}},
			wantErr: "line range 1-4 is out of bounds",
		},
		{
			name:        "returns a canceled context",
			ctx:         canceledCtx,
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{{
				From:    hashlineRef(2, "beta"),
				To:      hashlineRef(2, "beta"),
				Content: new("BETA"),
			}},
			wantErrIs: context.Canceled,
		},
		{
			name:        "validates all edits before applying",
			fileContent: "alpha\nbeta\ngamma",
			edits: []FlatEdit{
				{
					From:    hashlineRef(1, "alpha"),
					To:      hashlineRef(1, "alpha"),
					Content: new("ALPHA"),
				},
				{
					From:    fmt.Sprintf("3#%s", staleGammaHash),
					To:      hashlineRef(3, "gamma"),
					Content: new("ALPHA"),
				},
			},
			wantMismatch: &HashMismatch{Line: 3, Expected: staleGammaHash, Actual: gammaHash},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			got, _, _, err := ApplyHashlineEdit(ctx, tt.fileContent, EditInput{Edits: tt.edits})

			switch {
			case tt.wantErrIs != nil:
				require.ErrorIs(t, err, tt.wantErrIs)
				require.Empty(t, got)
			case tt.wantMismatch != nil:
				require.Error(t, err)
				require.Empty(t, got)
				var mismatchErr *HashlineMismatchError
				require.ErrorAs(t, err, &mismatchErr)
				require.Equal(t, []HashMismatch{*tt.wantMismatch}, mismatchErr.mismatches)
			case tt.wantErr != "":
				require.Error(t, err)
				require.Empty(t, got)
				require.Contains(t, err.Error(), tt.wantErr)
			default:
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRunEditFileHash(t *testing.T) {
	original := "alpha\nbeta\ngamma"
	replacement := "BETA"
	edits := []FlatEdit{{
		From:    hashlineRef(2, "beta"),
		To:      hashlineRef(2, "beta"),
		Content: &replacement,
	}}

	tests := []struct {
		name    string
		hash    string // empty means "use the current file hash"
		wantErr string
		want    string
	}{
		{name: "matching file hash applies edit", want: "alpha\nBETA\ngamma"},
		{
			name: "leading hash sigil is stripped",
			hash: "#" + util.ComputeFileHash(original),
			want: "alpha\nBETA\ngamma",
		},
		{
			name: "full @file header is accepted",
			hash: "@file sample.txt#" + util.ComputeFileHash(original),
			want: "alpha\nBETA\ngamma",
		},
		{name: "stale file hash is rejected", hash: "DEAD", wantErr: "file TAG mismatch", want: original},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sample.txt")
			require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

			hash := tt.hash
			if hash == "" {
				hash = util.ComputeFileHash(original)
			}
			raw, err := json.Marshal(EditInput{Path: path, Hash: hash, Edits: edits})
			require.NoError(t, err)

			res, err := runEdit(t.Context(), raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, res.Content, "@file ")
				require.Contains(t, res.Content, "Re-read this file before another edit")
			}
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got))
		})
	}
}

func TestRunEditOutputIsTheDiffAlone(t *testing.T) {
	original := "alpha\nbeta\ngamma"
	replacement := "BETA"
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(original),
		Edits: []FlatEdit{{
			From:    hashlineRef(2, "beta"),
			To:      hashlineRef(2, "beta"),
			Content: &replacement,
		}},
	})
	require.NoError(t, err)

	res, err := runEdit(t.Context(), raw)
	require.NoError(t, err)
	require.Contains(t, res.Output, "-beta")
	require.Contains(t, res.Output, "+BETA")
	require.NotContains(t, res.Output, "Re-read this file",
		"the re-read notice is model-facing; the diff card shows hunks only")
	require.Equal(t, path, res.Detail)
	require.Contains(t, res.Content, "Re-read this file before another edit")
}

func TestRunEditPreservesModeAndLeavesNoStagingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(original),
		Edits: []FlatEdit{{
			From:    hashlineRef(2, "beta"),
			To:      hashlineRef(2, "beta"),
			Content: new("BETA"),
		}},
	})
	require.NoError(t, err)

	_, err = runEdit(t.Context(), raw)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "an edit rewrites content, not permissions")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "a successful edit leaves no staging file behind")
}

func TestUnchangedRevisionGuard(t *testing.T) {
	original := "alpha\nbeta\ngamma"
	guard := unchangedRevisionGuard(util.RevisionOf(original), "sample.txt")

	require.NoError(t, guard([]byte(original)))
	require.NoError(t, guard([]byte("alpha\r\nbeta\r\ngamma")), "line endings are normalized like the read path")

	err := guard([]byte("someone else got here first"))
	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.ChangedDuringEditCode, refusal.Code)
	require.Contains(t, err.Error(), "file changed during edit")
	require.Contains(t, err.Error(), "reapply the edit onto the new content")
}

func hashlineRef(line int, content string) string {
	return fmt.Sprintf("%d#%s", line, util.ComputeLineHash(content))
}

func TestNormalizeFileTag(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"A1B2", "A1B2"},
		{"a1b2", "A1B2"},
		{"#A1B2", "A1B2"},
		{"@file src/app.py#A1B2", "A1B2"},
		{"  @file src/app.py#a1b2  ", "A1B2"},
		{"", ""},
		{"#", ""},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, normalizeFileTag(tt.in), tt.in)
	}
}

func TestParseLineRef(t *testing.T) {
	line, hash, err := parseLineRef("5#abc|content")
	require.NoError(t, err)
	require.Equal(t, 5, line)
	require.Equal(t, "abc", hash)

	_, _, err = parseLineRef("1#pix|.idea/\n2#qwr|/cozyphi")
	require.Error(t, err)
	require.Contains(t, err.Error(), "single LINE#HASH")
}

func TestRunEditRejectsOverlappingRanges(t *testing.T) {
	original := "alpha\nbeta\ngamma\ndelta\nepsilon"
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	for _, tt := range []struct {
		name       string
		edits      []FlatEdit
		wantErr    string
		wantRanges []string
		want       string
	}{
		{
			name: "partial overlap at line 3",
			edits: []FlatEdit{
				{From: hashlineRef(2, "beta"), To: hashlineRef(3, "gamma"), Content: new("X")},
				{From: hashlineRef(3, "gamma"), To: hashlineRef(4, "delta"), Content: new("Y")},
			},
			wantErr:    "overlap",
			wantRanges: []string{"2-3", "3-4"},
			want:       original,
		},
		{
			name: "nested range",
			edits: []FlatEdit{
				{From: hashlineRef(1, "alpha"), To: hashlineRef(4, "delta"), Content: new("X")},
				{From: hashlineRef(2, "beta"), To: hashlineRef(3, "gamma"), Content: new("Y")},
			},
			wantErr:    "overlap",
			wantRanges: []string{"1-4", "2-3"},
			want:       original,
		},
		{
			name: "adjacent ranges apply in order",
			edits: []FlatEdit{
				{From: hashlineRef(2, "beta"), To: hashlineRef(2, "beta"), Content: new("B")},
				{From: hashlineRef(3, "gamma"), To: hashlineRef(3, "gamma"), Content: new("G")},
			},
			want: "alpha\nB\nG\ndelta\nepsilon",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh file per case: a case that (wrongly) succeeds must not
			// poison the file tag for the next one.
			require.NoError(t, os.WriteFile(path, []byte(original), 0o644))
			raw, err := json.Marshal(EditInput{
				Path:  path,
				Hash:  util.ComputeFileHash(original),
				Edits: tt.edits,
			})
			require.NoError(t, err)

			_, err = runEdit(t.Context(), raw)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				// Name both offending ranges so the caller can fix the call.
				for _, r := range tt.wantRanges {
					require.Contains(t, err.Error(), r)
				}
				require.Contains(t, err.Error(), "re-read the file between edits",
					"the refusal must tell the caller how to split the call")
			} else {
				require.NoError(t, err)
			}

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(got), "rejected edits must leave the file untouched")
		})
	}
}

func differentHash(current string) string {
	if current == "aaa" {
		return "aab"
	}
	return "aaa"
}

// ---- Re-anchoring through the authorized path ----

// The model numbered the second range as if its own first edit had already
// grown the file. The resolver shifts the anchors back onto the observed
// lines, applies the edit there, and reports the correction in the result.
func TestRunAuthorizedEditReanchorsShiftedRanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma\ndelta\nepsilon\nzeta"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{
		hashlineRef(2, "beta"), hashlineRef(3, "gamma"),
		hashlineRef(5, "epsilon"), hashlineRef(6, "zeta"),
	})

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(original),
		Edits: []FlatEdit{
			{From: hashlineRef(2, "beta"), To: hashlineRef(3, "gamma"), Content: new("combined")},
			// claimed three lines down, as if the first edit had grown the file
			{From: hashlineRef(8, "epsilon"), To: hashlineRef(9, "zeta"), Content: new("EPSILON")},
		},
	})
	require.NoError(t, err)

	res, err := runAuthorizedEdit(t.Context(), raw, ledger)
	require.NoError(t, err)
	require.Contains(t, res.Content, "rebased edits[1] from 8-9 to 5-6 (delta -3)")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\ncombined\ndelta\nEPSILON", string(got))
}

// Exact anchors keep today's behavior through the authorized path: the edit
// applies at the claimed lines and the body carries no rebase notice.
func TestRunAuthorizedEditExactAnchors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{
		hashlineRef(2, "beta"), hashlineRef(3, "gamma"),
	})

	raw, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  util.ComputeFileHash(original),
		Edits: []FlatEdit{{From: hashlineRef(2, "beta"), To: hashlineRef(3, "gamma"), Content: new("combined")}},
	})
	require.NoError(t, err)

	res, err := runAuthorizedEdit(t.Context(), raw, ledger)
	require.NoError(t, err)
	require.NotContains(t, res.Content, "rebased")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\ncombined", string(got))
}

// Two identical lines make the claimed shift a guess; the refusal names the
// ambiguity and the file stays as it was.
func TestRunAuthorizedEditRefusesAmbiguousShift(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\ntwin\ngamma\ntwin\ndelta"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{
		hashlineRef(2, "twin"), hashlineRef(4, "twin"), hashlineRef(5, "delta"),
	})

	raw, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  util.ComputeFileHash(original),
		Edits: []FlatEdit{{From: hashlineRef(3, "twin"), To: hashlineRef(5, "delta"), Content: new("X")}},
	})
	require.NoError(t, err)

	_, err = runAuthorizedEdit(t.Context(), raw, ledger)
	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.AmbiguousReanchor.Code(), refusal.Code)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(got), "a refused edit leaves the file as it was")
}

// ---- Successor capability ----

// The final coordinates of each replacement: a lower edit shifts every
// higher one, so two replacements must land where the merged content puts
// them, not where their old line numbers were.
func TestApplyHashlineEditReportsNewSpans(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive\nsix"
	// Replace 5-6 (two lines) with three, and 2-3 (two lines) with one.
	got, _, spans, err := ApplyHashlineEdit(t.Context(), content, EditInput{Edits: []FlatEdit{
		{From: "5#" + util.ComputeLineHash("five"), To: "6#" + util.ComputeLineHash("six"), Content: new("A\nB\nC")},
		{From: "2#" + util.ComputeLineHash("two"), To: "3#" + util.ComputeLineHash("three"), Content: new("X")},
	}})
	require.NoError(t, err)
	require.Equal(t, "one\nX\nfour\nA\nB\nC", got)
	require.Equal(t, []editledger.Span{{From: 2, To: 2}, {From: 4, To: 6}}, spans)
}

// A deletion's span is the empty gap (s, s-1); the successor window is the
// context around it, not a negative range.
func TestApplyHashlineEditDeletionSpan(t *testing.T) {
	content := "one\ntwo\nthree\nfour"
	got, _, spans, err := ApplyHashlineEdit(t.Context(), content, EditInput{Edits: []FlatEdit{
		{From: "2#" + util.ComputeLineHash("two"), To: "2#" + util.ComputeLineHash("two")},
	}})
	require.NoError(t, err)
	require.Equal(t, "one\nthree\nfour", got)
	require.Equal(t, []editledger.Span{{From: 2, To: 1}}, spans)
}

// The anchors an authorized edit prints are real: they hash the new file's
// lines and authorize a second edit without any read in between.
func TestAuthorizedEditMintsSuccessorGrant(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma\ndelta\nepsilon"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{
		hashlineRef(2, "beta"), hashlineRef(3, "gamma"),
	})

	replacement := "BETA"
	raw, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  util.ComputeFileHash(original),
		Edits: []FlatEdit{{From: hashlineRef(2, "beta"), To: hashlineRef(2, "beta"), Content: &replacement}},
	})
	require.NoError(t, err)

	res, err := runAuthorizedEdit(t.Context(), raw, ledger)
	require.NoError(t, err)
	require.Contains(t, res.Content, "authorize the next edit")
	require.NotContains(t, res.Content, "Re-read this file")

	// Every printed anchor is live in the grant: a second edit of the same
	// region needs no read in between.
	newContent := "alpha\nBETA\ngamma\ndelta\nepsilon"
	newTag := util.ComputeFileHash(newContent)
	second := fmt.Sprintf("%d#%s", 4, util.ComputeLineHash("delta"))
	claim, resolution := ledger.Claim(path, newTag, []editledger.Ref{
		{Line: 4, Hash: util.ComputeLineHash("delta")},
		{Line: 4, Hash: util.ComputeLineHash("delta")},
	})
	require.False(t, resolution.Outcome.Refused(), "successor grant must cover the changed region's context")
	require.Equal(t, editledger.Span{From: 4, To: 4}, resolution.Lines[0])
	ledger.Release(claim)

	// The printed body names the exact anchors it minted.
	require.Contains(t, res.Content, second)
	require.Contains(t, res.Content, "hash="+newTag)

	// The old TAG is dead: a replay of the first edit refuses typed.
	_, resolution = ledger.Claim(path, util.ComputeFileHash(original), []editledger.Ref{
		{Line: 2, Hash: util.ComputeLineHash("beta")},
		{Line: 2, Hash: util.ComputeLineHash("beta")},
	})
	require.Equal(t, editledger.SnapshotConsumed, resolution.Outcome)
}

// Without a ledger there is no capability to print: the ledger-less path
// keeps the honest re-read instruction instead of anchors that authorize
// nothing.
func TestLedgerLessEditKeepsReReadMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "alpha\nbeta\ngamma"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(original),
		Edits: []FlatEdit{{
			From: hashlineRef(2, "beta"), To: hashlineRef(2, "beta"), Content: new("BETA"),
		}},
	})
	require.NoError(t, err)

	res, err := runEdit(t.Context(), raw)
	require.NoError(t, err)
	require.Contains(t, res.Content, "Re-read this file before another edit")
	require.NotContains(t, res.Content, "authorize the next edit")
}

// A replacement large enough to run the union past the grant cap truncates
// the grant from the top and says so: what stayed outside must be re-read.
func TestSuccessorGrantTruncatesAtCap(t *testing.T) {
	lines := make([]string, 700)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%03d", i+1)
	}
	spans := []editledger.Span{{From: 100, To: 699}}
	grant := successorGrantFor(spans, lines, "AB12")
	require.Len(t, grant.anchors, maxGeneratedGrantAnchors)
	require.True(t, grant.capped)
	require.Equal(t, fmt.Sprintf("75#%s", util.ComputeLineHash("line-075")), grant.anchors[0])

	var body strings.Builder
	writeSuccessorBlock(&body, grant)
	out := body.String()
	require.Contains(
		t,
		out,
		"+"+strconv.Itoa(maxGeneratedGrantAnchors-maxDisplayedAnchors)+" more live anchors not shown",
	)
	require.Contains(t, out, "beyond them read with mode")
	// The display starts at the changed region, not at the grant's first
	// context line: what the edit touched outranks what merely surrounds it.
	require.Contains(t, out, fmt.Sprintf("hash=AB12; all prior anchors are invalid:\n%d#%s ", spans[0].From,
		util.ComputeLineHash(lines[spans[0].From-1])))
}

// A concurrent writer that lands between the read and the swap kills the
// edit (changed_during_edit) and must mint nothing: the old claim is handed
// back — the read still describes a file the edit never touched — and the
// would-be successor authorizes nothing.
func TestChangedDuringEditMintsNoSuccessor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "one\ntwo\nthree"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{hashlineRef(2, "two")})

	replacement := "TWO!"
	raw, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  util.ComputeFileHash(original),
		Edits: []FlatEdit{{From: hashlineRef(2, "two"), To: hashlineRef(2, "two"), Content: &replacement}},
	})
	require.NoError(t, err)

	foreign := "someone else got here first\n"
	ctx := tooldef.WithMutationGuard(t.Context(), func(context.Context, string) error {
		return os.WriteFile(path, []byte(foreign), 0o644)
	})
	_, err = runAuthorizedEdit(ctx, raw, ledger)
	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.ChangedDuringEditCode, refusal.Code)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, foreign, string(got), "the concurrent writer's content survives")

	// The failed edit released its claim: the original read authorizes again.
	claim, resolution := ledger.Claim(path, util.ComputeFileHash(original), []editledger.Ref{
		{Line: 2, Hash: util.ComputeLineHash("two")},
		{Line: 2, Hash: util.ComputeLineHash("two")},
	})
	require.Equal(t, editledger.Granted, resolution.Outcome)
	ledger.Release(claim)

	// The would-be successor answers nothing: the grant that never landed is
	// not in the ledger under any tag.
	wouldBe := "one\nTWO!\nthree"
	_, resolution = ledger.Claim(path, util.ComputeFileHash(wouldBe), []editledger.Ref{
		{Line: 2, Hash: util.ComputeLineHash("TWO!")},
		{Line: 2, Hash: util.ComputeLineHash("TWO!")},
	})
	require.Equal(t, editledger.NoCapability, resolution.Outcome)
}

// A writer that lands on the target after the pre-swap guard has run must not
// be clobbered: the swap re-reads the file immediately before the rename, so
// the foreign content survives and the edit is refused.
func TestEditRefusesWriterLandingAfterVerify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	original := "one\ntwo\nthree"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

	ledger := editledger.New()
	ledger.Authorize(path, util.RevisionOf(original), []string{hashlineRef(2, "two")})

	replacement := "TWO!"
	raw, err := json.Marshal(EditInput{
		Path:  path,
		Hash:  util.ComputeFileHash(original),
		Edits: []FlatEdit{{From: hashlineRef(2, "two"), To: hashlineRef(2, "two"), Content: &replacement}},
	})
	require.NoError(t, err)

	// The writer lands on the second guard call — after the swap's own
	// pre-rename checks have all passed.
	foreign := "someone else got here first\n"
	calls := 0
	ctx := tooldef.WithMutationGuard(t.Context(), func(context.Context, string) error {
		calls++
		if calls < 2 {
			return nil
		}
		return os.WriteFile(path, []byte(foreign), 0o644)
	})

	_, err = EditTool(ledger).Run(ctx, raw)

	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.ChangedDuringEditCode, refusal.Code)
	got, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, foreign, string(got), "the writer that landed last must survive")
}

// ---- Display TAG collisions ----

// collidingContents returns two file texts whose display TAGs are equal and
// whose revision identities differ. Only the middle line changes, so every
// anchor the first read authorized still validates against the second text:
// nothing but the full revision can tell the two apart.
func collidingContents(t *testing.T) (first, second string) {
	t.Helper()
	first = "start\nexternal_value_0\nend\n"
	firstRev := util.RevisionOf(first)
	for i := 1; i < 1<<17; i++ {
		second = fmt.Sprintf("start\nexternal_value_%d\nend\n", i)
		rev := util.RevisionOf(second)
		if rev == firstRev || rev.Tag() != firstRev.Tag() {
			continue
		}
		return first, second
	}
	t.Fatal("no TAG collision found in the search range")
	return "", ""
}

// authorizeAsEditableRead grants exactly what read with mode:"edit" grants:
// one anchor per split line, keyed by the full revision of the text read.
func authorizeAsEditableRead(ledger *editledger.Ledger, path, text string) {
	lines := strings.Split(util.NormalizeLF(text), "\n")
	anchors := make([]string, 0, len(lines))
	for i, line := range lines {
		anchors = append(anchors, fmt.Sprintf("%d#%s", i+1, util.ComputeLineHash(line)))
	}
	ledger.Authorize(path, util.RevisionOf(text), anchors)
}

// An external writer replaced the file with different content that happens to
// hash to the same 4-hex display TAG. The anchors still match line by line and
// the TAG the model quotes still matches the file on disk, so only the full
// revision identity can refuse this: the edit must not land.
func TestEditRefusesCollidingTagWithChangedContent(t *testing.T) {
	first, second := collidingContents(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	require.NoError(t, os.WriteFile(path, []byte(first), 0o644))

	ledger := editledger.New()
	authorizeAsEditableRead(ledger, path, first)

	// The external swap: same TAG, different content.
	require.NoError(t, os.WriteFile(path, []byte(second), 0o644))
	require.Equal(t, util.ComputeFileHash(first), util.ComputeFileHash(second))

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(first),
		Edits: []FlatEdit{{
			From: hashlineRef(1, "start"), To: hashlineRef(3, "end"), Content: new("REPLACED"),
		}},
	})
	require.NoError(t, err)

	_, err = EditTool(ledger).Run(t.Context(), raw)
	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.TagChangedCode, refusal.Code)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, second, string(got), "the external writer's content must survive")
}

// The same collision arriving after the disk check but before the swap: the
// pre-swap Verify guard compares revisions too, so a same-TAG replacement
// still refuses instead of being overwritten.
func TestVerifyGuardRefusesSameTagSwap(t *testing.T) {
	first, second := collidingContents(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	require.NoError(t, os.WriteFile(path, []byte(first), 0o644))

	ledger := editledger.New()
	authorizeAsEditableRead(ledger, path, first)

	raw, err := json.Marshal(EditInput{
		Path: path,
		Hash: util.ComputeFileHash(first),
		Edits: []FlatEdit{{
			From: hashlineRef(2, "external_value_0"), To: hashlineRef(2, "external_value_0"),
			Content: new("REPLACED"),
		}},
	})
	require.NoError(t, err)

	// The mutation guard runs inside the atomic write; the invocation that
	// precedes the pre-swap Verify is the one that can plant the collision.
	swapped := false
	ctx := tooldef.WithMutationGuard(t.Context(), func(context.Context, string) error {
		if swapped {
			return nil
		}
		swapped = true
		return os.WriteFile(path, []byte(second), 0o644)
	})

	_, err = runAuthorizedEdit(ctx, raw, ledger)
	require.Error(t, err)
	var refusal *EditRefusal
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, editledger.ChangedDuringEditCode, refusal.Code)
	require.Contains(t, refusal.What, "still shows TAG")

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, second, string(got), "the concurrent writer's content must survive")
}

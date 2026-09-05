package editledger

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// ref builds one claimed endpoint; a shorthand so the range tables below
// read as what they authorize.
func ref(line int, hash string) Ref {
	return Ref{Line: line, Hash: hash}
}

func TestLedgerClaimsExactAuthorizedAnchors(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"})

	claim, resolution := ledger.Claim("/work/sample.txt", "a1b2", []Ref{ref(2, "abc"), ref(3, "def")})
	require.Equal(t, Granted, resolution.Outcome)
	require.NotNil(t, claim)
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(3, "def")})
	require.Equal(t, SnapshotConsumed, resolution.Outcome, "an applied edit ends the authorization")
}

func TestLedgerDoesNotCombineSeparateReturnedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"4#ghi"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(4, "ghi")})
	require.Equal(t, MixedGrants, resolution.Outcome)
}

// A refused claim must cost nothing: the file was not touched, so the read
// that authorized it still describes it.
func TestLedgerRefusedClaimKeepsAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(1, "xyz"), ref(1, "xyz")})
	require.Equal(t, AnchorNotObserved, resolution.Outcome, "an anchor that was never returned is not authorized")
	_, resolution = ledger.Claim("/work/sample.txt", "DEAD", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, NoCapability, resolution.Outcome, "a wrong tag is not authorized")

	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "the correct retry still works without a re-read")
}

// An edit that failed to apply gives the authorization back, and the
// consumed disposition goes away with it: the snapshot is live again.
func TestLedgerReleaseRestoresAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	claim, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	ledger.Release(claim)

	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
}

func TestLedgerClaimIsExclusive(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if _, resolution := ledger.Claim(
				"/work/sample.txt",
				"A1B2",
				[]Ref{ref(2, "abc"), ref(2, "abc")},
			); resolution.Outcome == Granted {
				successes.Add(1)
			}
		})
	}
	wg.Wait()

	require.Equal(t, int32(1), successes.Load())
}

// A session that reads far more files than it edits must not accumulate every
// snapshot it ever authorized.
func TestLedgerEvictsOldestSnapshots(t *testing.T) {
	ledger := New()
	for i := range maxTrackedSnapshots + 4 {
		ledger.Authorize(fmt.Sprintf("/work/file%d.txt", i), "A1B2", []string{"2#abc"})
	}

	require.LessOrEqual(t, len(ledger.grants), maxTrackedSnapshots)
	_, resolution := ledger.Claim("/work/file0.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, SnapshotEvicted, resolution.Outcome, "the oldest read is evicted first")
	_, resolution = ledger.Claim(
		fmt.Sprintf("/work/file%d.txt", maxTrackedSnapshots+3),
		"A1B2",
		[]Ref{ref(2, "abc"), ref(2, "abc")},
	)
	require.Equal(t, Granted, resolution.Outcome, "the newest read stays authorized")
}

// Repeated editable reads of one file must not pile up grants either.
func TestLedgerCapsGrantsPerSnapshot(t *testing.T) {
	ledger := New()
	for i := range maxGrantsPerSnapshot + 3 {
		ledger.Authorize("/work/sample.txt", "A1B2", []string{fmt.Sprintf("%d#abc", i+1)})
	}

	require.Len(t, ledger.grants[snapshotKey("/work/sample.txt", "A1B2")], maxGrantsPerSnapshot)
}

func TestLedgerRefusesMalformedRefs(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc")})
	require.Equal(t, InvalidRef, resolution.Outcome, "an unpaired anchor is not a range")
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", nil)
	require.Equal(t, InvalidRef, resolution.Outcome, "an empty request edits nothing")
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(0, "abc"), ref(2, "abc")})
	require.Equal(t, InvalidRef, resolution.Outcome, "a line number below one is refused, not treated as unseen")
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abcd"), ref(2, "abc")})
	require.Equal(t, InvalidRef, resolution.Outcome, "a wrong-length hash is refused, not treated as unseen")
}

// A re-read of the same content returns the same TAG; it must revive a tag a
// previous applied edit had consumed.
func TestLedgerReauthorizeRevivesConsumedTag(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, SnapshotConsumed, resolution.Outcome)

	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "a fresh read of the same revision re-authorizes it")
}

// The dispositions ring is bounded: once a dead snapshot rotates out, its tag
// reports a plain no_capability again.
func TestLedgerDispositionsAreBounded(t *testing.T) {
	ledger := New()
	for i := range maxRememberedDispositions + 2 {
		ledger.Authorize(fmt.Sprintf("/work/file%d.txt", i), fmt.Sprintf("T%02dX", i), []string{"2#abc"})
		_, resolution := ledger.Claim(
			fmt.Sprintf("/work/file%d.txt", i),
			fmt.Sprintf("T%02dX", i),
			[]Ref{ref(2, "abc"), ref(2, "abc")},
		)
		require.Equal(t, Granted, resolution.Outcome)
	}

	_, resolution := ledger.Claim("/work/file0.txt", "T00X", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, NoCapability, resolution.Outcome, "the oldest dead tag rotated out of the ring")
	_, resolution = ledger.Claim(
		fmt.Sprintf("/work/file%d.txt", maxRememberedDispositions+1),
		fmt.Sprintf("T%02dX", maxRememberedDispositions+1),
		[]Ref{ref(2, "abc"), ref(2, "abc")},
	)
	require.Equal(t, SnapshotConsumed, resolution.Outcome, "the newest dead tag still names its reason")
}

// The stable codes are the contract the analyzer and the tool boundary share.
func TestOutcomeCodes(t *testing.T) {
	cases := map[Outcome]string{
		Granted:           "granted",
		NoCapability:      "no_capability",
		SnapshotConsumed:  "snapshot_consumed",
		SnapshotEvicted:   "snapshot_evicted",
		AnchorNotObserved: "anchor_not_observed",
		MixedGrants:       "mixed_grants",
		AmbiguousReanchor: "ambiguous_reanchor",
		InvalidRef:        "invalid_ref",
	}
	for outcome, code := range cases {
		require.Equal(t, code, outcome.Code())
	}
}

// ---- Re-anchoring ----

// The typical multi-edit failure: the model numbers later ranges as if its own
// earlier edit had already applied, shifting lines down. A uniform shift whose
// hashes occur exactly once inside one grant re-anchors onto the observed lines.
func TestLedgerReanchorsShiftDown(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "5#def"})

	claim, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(8, "def")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, -3, resolution.Delta, "the claimed numbers sit three lines below the observed ones")
	require.Equal(t, [][2]int{{2, 5}}, resolution.Lines)
	require.NotNil(t, claim)
}

// The same failure in the other direction: the model removed lines above in
// its own numbering, so the claimed lines sit above the observed ones.
func TestLedgerReanchorsShiftUp(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"12#ghi", "15#jkl"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(9, "ghi"), ref(12, "jkl")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, 3, resolution.Delta)
	require.Equal(t, [][2]int{{12, 15}}, resolution.Lines)
}

// Unshifted exact ranges may coexist with shifted ones in one call.
func TestLedgerRebaseAllowsExactAndShiftedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "5#def", "9#ghi", "15#jkl"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{
		ref(2, "abc"), ref(5, "def"), // exact range
		ref(9, "ghi"), ref(12, "jkl"), // shifted: 9 exact, 12 -> 15
	})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, 3, resolution.Delta)
	require.Equal(t, [][2]int{{2, 5}, {9, 15}}, resolution.Lines)
}

// The exact line always wins: an anchor that matches its own line is never
// moved to another occurrence of the same hash.
func TestLedgerExactBeatsShift(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"5#abc", "9#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(9, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, 0, resolution.Delta)
	require.Equal(t, [][2]int{{5, 9}}, resolution.Lines)
}

// A duplicate hash under the claimed shift names the ambiguity instead of
// guessing, and endpoints that disagree on the shift stay refused too.
func TestLedgerRefusesAmbiguousShift(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "7#abc", "9#def"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(9, "def")})
	require.Equal(
		t,
		AmbiguousReanchor,
		resolution.Outcome,
		"a hash matching two candidate lines is a guess, not a grant",
	)

	ledger = New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"12#ghi", "14#jkl"})
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(9, "ghi"), ref(12, "jkl")})
	require.Equal(t, AmbiguousReanchor, resolution.Outcome, "endpoints moving by different shifts stay refused")
}

// A shift whose endpoints only resolve across two different reads is still
// two reads spliced together, exactly like the unshifted case.
func TestLedgerShiftAcrossGrantsIsMixed(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"9#def"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(8, "def")})
	require.Equal(t, MixedGrants, resolution.Outcome)
}

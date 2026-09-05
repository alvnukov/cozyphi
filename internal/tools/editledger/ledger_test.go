package editledger

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/util"
)

// ref builds one claimed endpoint; a shorthand so the range tables below
// read as what they authorize.
func ref(line int, hash string) Ref {
	return Ref{Line: line, Hash: hash}
}

// revOf builds the revision whose display TAG is exactly these 4 hex chars,
// so the tables below keep reading in the tags an edit call would quote.
func revOf(tag string) util.Revision {
	n, err := strconv.ParseUint(tag, 16, 16)
	if err != nil {
		panic("test tags are 4 hex chars: " + tag)
	}
	return util.Revision(n)
}

// altRev is a different revision showing the same display TAG as revOf(tag):
// the 65536-way collision the ledger must not let anchors cross.
func altRev(tag string) util.Revision {
	return revOf(tag) | 1<<32
}

func TestLedgerClaimsExactAuthorizedAnchors(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc", "3#def"})

	claim, resolution := ledger.Claim("/work/sample.txt", "a1b2", []Ref{ref(2, "abc"), ref(3, "def")})
	require.Equal(t, Granted, resolution.Outcome)
	require.NotNil(t, claim)
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(3, "def")})
	require.Equal(t, SnapshotConsumed, resolution.Outcome, "an applied edit ends the authorization")
}

func TestLedgerDoesNotCombineSeparateReturnedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"4#ghi"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(4, "ghi")})
	require.Equal(t, MixedGrants, resolution.Outcome)
}

// A refused claim must cost nothing: the file was not touched, so the read
// that authorized it still describes it.
func TestLedgerRefusedClaimKeepsAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

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
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

	claim, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	ledger.Release(claim)

	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
}

func TestLedgerClaimIsExclusive(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

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
		ledger.Authorize(fmt.Sprintf("/work/file%d.txt", i), revOf("A1B2"), []string{"2#abc"})
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
		ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{fmt.Sprintf("%d#abc", i+1)})
	}

	require.Len(t, ledger.grants[snapshotKey("/work/sample.txt", revOf("A1B2"))], maxGrantsPerSnapshot)
}

func TestLedgerRefusesMalformedRefs(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

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
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, SnapshotConsumed, resolution.Outcome)

	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "a fresh read of the same revision re-authorizes it")
}

// The dispositions ring is bounded: once a dead snapshot rotates out, its tag
// reports a plain no_capability again.
func TestLedgerDispositionsAreBounded(t *testing.T) {
	ledger := New()
	for i := range maxRememberedDispositions + 2 {
		tag := fmt.Sprintf("%04X", i)
		ledger.Authorize(fmt.Sprintf("/work/file%d.txt", i), revOf(tag), []string{"2#abc"})
		_, resolution := ledger.Claim(
			fmt.Sprintf("/work/file%d.txt", i),
			tag,
			[]Ref{ref(2, "abc"), ref(2, "abc")},
		)
		require.Equal(t, Granted, resolution.Outcome)
	}

	_, resolution := ledger.Claim("/work/file0.txt", "0000", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, NoCapability, resolution.Outcome, "the oldest dead tag rotated out of the ring")
	_, resolution = ledger.Claim(
		fmt.Sprintf("/work/file%d.txt", maxRememberedDispositions+1),
		fmt.Sprintf("%04X", maxRememberedDispositions+1),
		[]Ref{ref(2, "abc"), ref(2, "abc")},
	)
	require.Equal(t, SnapshotConsumed, resolution.Outcome, "the newest dead tag still names its reason")
}

// A write replaces the file, so every earlier snapshot of the path dies with
// it and names the write as its reason; other paths keep their snapshots, the
// written revision is live, and a fresh read of a superseded revision revives
// it like any other dead one.
func TestLedgerSupersedeRetiresEverySnapshotOfPath(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", revOf("C3D4"), []string{"2#abc"})
	ledger.Authorize("/work/other.txt", revOf("A1B2"), []string{"2#abc"})

	ledger.Supersede("/work/sample.txt", revOf("E5F6"), []string{"2#xyz"})

	ledger.mu.Lock()
	for _, tag := range []string{"A1B2", "C3D4"} {
		key := snapshotKey("/work/sample.txt", revOf(tag))
		require.NotContains(t, ledger.grants, key, "the pre-write snapshot %s is gone", tag)
		require.Equal(
			t, SnapshotSuperseded, ledger.dispositions[key],
			"the pre-write snapshot %s names its reason", tag,
		)
	}
	require.Contains(t, ledger.grants, snapshotKey("/work/sample.txt", revOf("E5F6")))
	require.Contains(t, ledger.grants, snapshotKey("/work/other.txt", revOf("A1B2")))
	ledger.mu.Unlock()

	for _, tag := range []string{"A1B2", "C3D4"} {
		_, resolution := ledger.Claim("/work/sample.txt", tag, []Ref{ref(2, "abc"), ref(2, "abc")})
		require.Equal(t, SnapshotSuperseded, resolution.Outcome, "an edit quoting the pre-write TAG %s", tag)
	}
	claim, resolution := ledger.Claim("/work/sample.txt", "E5F6", []Ref{ref(2, "xyz"), ref(2, "xyz")})
	require.Equal(t, Granted, resolution.Outcome, "the written revision authorizes the next edit")
	ledger.Release(claim)
	_, resolution = ledger.Claim("/work/other.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "another path keeps its snapshot")

	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "a fresh read of the superseded revision revives it")
}

// Rewriting the same content supersedes the revision with itself: it stays
// live, carrying only the grant the write result printed.
func TestLedgerSupersedeSameRevisionStaysLive(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"9#old"})

	ledger.Supersede("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(9, "old"), ref(9, "old")})
	require.Equal(t, AnchorNotObserved, resolution.Outcome, "the pre-write grant died with the write")
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, Granted, resolution.Outcome, "the write's own anchors are live")
}

// The stable codes are the contract the analyzer and the tool boundary share.
func TestOutcomeCodes(t *testing.T) {
	cases := map[Outcome]string{
		Granted:            "granted",
		NoCapability:       "no_capability",
		SnapshotConsumed:   "snapshot_consumed",
		SnapshotEvicted:    "snapshot_evicted",
		SnapshotSuperseded: "snapshot_superseded",
		AnchorNotObserved:  "anchor_not_observed",
		MixedGrants:        "mixed_grants",
		AmbiguousReanchor:  "ambiguous_reanchor",
		InvalidRef:         "invalid_ref",
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
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc", "5#def"})

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
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"12#ghi", "15#jkl"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(9, "ghi"), ref(12, "jkl")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, 3, resolution.Delta)
	require.Equal(t, [][2]int{{12, 15}}, resolution.Lines)
}

// Unshifted exact ranges may coexist with shifted ones in one call.
func TestLedgerRebaseAllowsExactAndShiftedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc", "5#def", "9#ghi", "15#jkl"})

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
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"5#abc", "9#abc"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(9, "abc")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, 0, resolution.Delta)
	require.Equal(t, [][2]int{{5, 9}}, resolution.Lines)
}

// A duplicate hash under the claimed shift names the ambiguity instead of
// guessing, and endpoints that disagree on the shift stay refused too.
func TestLedgerRefusesAmbiguousShift(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc", "7#abc", "9#def"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(9, "def")})
	require.Equal(
		t,
		AmbiguousReanchor,
		resolution.Outcome,
		"a hash matching two candidate lines is a guess, not a grant",
	)

	ledger = New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"12#ghi", "14#jkl"})
	_, resolution = ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(9, "ghi"), ref(12, "jkl")})
	require.Equal(t, AmbiguousReanchor, resolution.Outcome, "endpoints moving by different shifts stay refused")
}

// A shift whose endpoints only resolve across two different reads is still
// two reads spliced together, exactly like the unshifted case.
func TestLedgerShiftAcrossGrantsIsMixed(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"9#def"})

	_, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(5, "abc"), ref(8, "def")})
	require.Equal(t, MixedGrants, resolution.Outcome)
}

// Commit settles an applied claim: the successor grant answers for the new
// revision, and the old TAG dies as consumed — a replay of the applied edit
// learns the typed reason instead of a bare no_capability.
func TestLedgerCommitMintsSuccessorAndKillsOldTag(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc", "3#bcd"})

	claim, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(3, "bcd")})
	require.Equal(t, Granted, resolution.Outcome)

	ledger.Commit(claim, revOf("C3D4"), []string{"2#xyz", "3#yzx"})

	_, successor := ledger.Claim("/work/sample.txt", "C3D4", []Ref{ref(2, "xyz"), ref(3, "yzx")})
	require.Equal(t, Granted, successor.Outcome)

	_, old := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(3, "bcd")})
	require.Equal(t, SnapshotConsumed, old.Outcome)
}

// A failed attempt hands the grant back, so the same anchors claim again;
// the ledger-less commit is a no-op, not a panic.
func TestLedgerCommitNilClaimIsNoop(t *testing.T) {
	ledger := New()
	ledger.Commit(nil, revOf("C3D4"), []string{"2#xyz"})

	_, resolution := ledger.Claim("/work/sample.txt", "C3D4", []Ref{ref(2, "xyz"), ref(2, "xyz")})
	require.Equal(t, NoCapability, resolution.Outcome)
}

// Two contents of one file can share a 4-hex display TAG. The tag the model
// quotes must name exactly one live revision: the newer editable read retires
// the older snapshot, and the older read's anchors go with it.
func TestLedgerRetiresOlderSnapshotSharingATag(t *testing.T) {
	first, second := revOf("A1B2"), altRev("A1B2")
	require.Equal(t, first.Tag(), second.Tag(), "the two revisions collide on the display TAG")
	require.NotEqual(t, first, second)

	ledger := New()
	ledger.Authorize("/work/sample.txt", first, []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", second, []string{"4#ghi"})

	_, stale := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Equal(t, AnchorNotObserved, stale.Outcome, "the retired revision's anchors authorize nothing")

	claim, resolution := ledger.Claim("/work/sample.txt", "A1B2", []Ref{ref(4, "ghi"), ref(4, "ghi")})
	require.Equal(t, Granted, resolution.Outcome)
	require.Equal(t, second, resolution.Revision, "the tag resolves to the surviving revision")
	require.Equal(t, second, claim.Revision(), "the claim carries the revision the edit must find on disk")
}

// A refused claim names no revision: there is nothing for the edit to verify.
func TestLedgerRefusedClaimCarriesNoRevision(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", revOf("A1B2"), []string{"2#abc"})

	claim, resolution := ledger.Claim("/work/sample.txt", "C3D4", []Ref{ref(2, "abc"), ref(2, "abc")})
	require.Nil(t, claim)
	require.Equal(t, NoCapability, resolution.Outcome)
	require.Zero(t, resolution.Revision)
	require.Zero(t, claim.Revision(), "a nil claim reports the zero revision instead of panicking")
}

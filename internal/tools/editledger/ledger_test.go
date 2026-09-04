package editledger

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLedgerClaimsExactAuthorizedAnchors(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"})

	claim, outcome := ledger.Claim("/work/sample.txt", "a1b2", []string{"2#abc", "3#def"})
	require.Equal(t, Granted, outcome)
	require.NotNil(t, claim)
	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"})
	require.Equal(t, SnapshotConsumed, outcome, "an applied edit ends the authorization")
}

func TestLedgerDoesNotCombineSeparateReturnedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"4#ghi"})

	_, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "4#ghi"})
	require.Equal(t, MixedGrants, outcome)
}

// A refused claim must cost nothing: the file was not touched, so the read
// that authorized it still describes it.
func TestLedgerRefusedClaimKeepsAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"1#xyz", "1#xyz"})
	require.Equal(t, AnchorNotObserved, outcome, "an anchor that was never returned is not authorized")
	_, outcome = ledger.Claim("/work/sample.txt", "DEAD", []string{"2#abc", "2#abc"})
	require.Equal(t, NoCapability, outcome, "a wrong tag is not authorized")

	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, Granted, outcome, "the correct retry still works without a re-read")
}

// An edit that failed to apply gives the authorization back, and the
// consumed disposition goes away with it: the snapshot is live again.
func TestLedgerReleaseRestoresAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	claim, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, Granted, outcome)
	ledger.Release(claim)

	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, Granted, outcome)
}

func TestLedgerClaimIsExclusive(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if _, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"}); outcome == Granted {
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
	_, outcome := ledger.Claim("/work/file0.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, SnapshotEvicted, outcome, "the oldest read is evicted first")
	_, outcome = ledger.Claim(
		fmt.Sprintf("/work/file%d.txt", maxTrackedSnapshots+3),
		"A1B2",
		[]string{"2#abc", "2#abc"},
	)
	require.Equal(t, Granted, outcome, "the newest read stays authorized")
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

	_, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc"})
	require.Equal(t, InvalidRef, outcome, "an unpaired anchor is not a range")
	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", nil)
	require.Equal(t, InvalidRef, outcome, "an empty request edits nothing")
	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"not-an-anchor", "2#abc"})
	require.Equal(t, InvalidRef, outcome, "a malformed anchor is refused, not treated as unseen")
}

// A re-read of the same content returns the same TAG; it must revive a tag a
// previous applied edit had consumed.
func TestLedgerReauthorizeRevivesConsumedTag(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, outcome := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, Granted, outcome)
	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, SnapshotConsumed, outcome)

	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	_, outcome = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.Equal(t, Granted, outcome, "a fresh read of the same revision re-authorizes it")
}

// The dispositions ring is bounded: once a dead snapshot rotates out, its tag
// reports a plain no_capability again.
func TestLedgerDispositionsAreBounded(t *testing.T) {
	ledger := New()
	for i := range maxRememberedDispositions + 2 {
		ledger.Authorize(fmt.Sprintf("/work/file%d.txt", i), fmt.Sprintf("T%02dX", i), []string{"2#abc"})
		_, outcome := ledger.Claim(
			fmt.Sprintf("/work/file%d.txt", i),
			fmt.Sprintf("T%02dX", i),
			[]string{"2#abc", "2#abc"},
		)
		require.Equal(t, Granted, outcome)
	}

	_, outcome := ledger.Claim("/work/file0.txt", "T00X", []string{"2#abc", "2#abc"})
	require.Equal(t, NoCapability, outcome, "the oldest dead tag rotated out of the ring")
	_, outcome = ledger.Claim(
		fmt.Sprintf("/work/file%d.txt", maxRememberedDispositions+1),
		fmt.Sprintf("T%02dX", maxRememberedDispositions+1),
		[]string{"2#abc", "2#abc"},
	)
	require.Equal(t, SnapshotConsumed, outcome, "the newest dead tag still names its reason")
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
		InvalidRef:        "invalid_ref",
	}
	for outcome, code := range cases {
		require.Equal(t, code, outcome.Code())
	}
}

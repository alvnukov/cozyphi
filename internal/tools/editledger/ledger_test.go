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

	_, ok := ledger.Claim("/work/sample.txt", "a1b2", []string{"2#abc", "3#def"})
	require.True(t, ok)
	_, ok = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"})
	require.False(t, ok, "an applied edit ends the authorization")
}

func TestLedgerDoesNotCombineSeparateReturnedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"4#ghi"})

	_, ok := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "4#ghi"})
	require.False(t, ok)
}

// A refused claim must cost nothing: the file was not touched, so the read
// that authorized it still describes it.
func TestLedgerRefusedClaimKeepsAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	_, ok := ledger.Claim("/work/sample.txt", "A1B2", []string{"1#xyz", "1#xyz"})
	require.False(t, ok, "an anchor that was never returned is not authorized")
	_, ok = ledger.Claim("/work/sample.txt", "DEAD", []string{"2#abc", "2#abc"})
	require.False(t, ok, "a wrong tag is not authorized")

	_, ok = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.True(t, ok, "the correct retry still works without a re-read")
}

// An edit that failed to apply gives the authorization back.
func TestLedgerReleaseRestoresAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	claim, ok := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.True(t, ok)
	ledger.Release(claim)

	_, ok = ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.True(t, ok)
}

func TestLedgerClaimIsExclusive(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if _, ok := ledger.Claim("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"}); ok {
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
	_, ok := ledger.Claim("/work/file0.txt", "A1B2", []string{"2#abc", "2#abc"})
	require.False(t, ok, "the oldest read is evicted first")
	_, ok = ledger.Claim(fmt.Sprintf("/work/file%d.txt", maxTrackedSnapshots+3), "A1B2", []string{"2#abc", "2#abc"})
	require.True(t, ok, "the newest read stays authorized")
}

// Repeated editable reads of one file must not pile up grants either.
func TestLedgerCapsGrantsPerSnapshot(t *testing.T) {
	ledger := New()
	for i := range maxGrantsPerSnapshot + 3 {
		ledger.Authorize("/work/sample.txt", "A1B2", []string{fmt.Sprintf("%d#abc", i+1)})
	}

	require.Len(t, ledger.grants[snapshotKey("/work/sample.txt", "A1B2")], maxGrantsPerSnapshot)
}

package editledger

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLedgerConsumesExactAuthorizedAnchors(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"})

	require.True(t, ledger.Consume("/work/sample.txt", "a1b2", []string{"2#abc", "3#def"}))
	require.False(t, ledger.Consume("/work/sample.txt", "A1B2", []string{"2#abc", "3#def"}))
}

func TestLedgerDoesNotCombineSeparateReturnedRanges(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"4#ghi"})

	require.False(t, ledger.Consume("/work/sample.txt", "A1B2", []string{"2#abc", "4#ghi"}))
}

func TestLedgerFailedAttemptConsumesSnapshotAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	require.False(t, ledger.Consume("/work/sample.txt", "A1B2", []string{"1#xyz", "1#xyz"}))
	require.False(t, ledger.Consume("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"}))
}

func TestLedgerWrongTagAttemptConsumesPathAuthorization(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	require.False(t, ledger.Consume("/work/sample.txt", "DEAD", []string{"2#abc", "2#abc"}))
	require.False(t, ledger.Consume("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"}))
}

func TestLedgerConsumeIsConcurrencySafe(t *testing.T) {
	ledger := New()
	ledger.Authorize("/work/sample.txt", "A1B2", []string{"2#abc"})

	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if ledger.Consume("/work/sample.txt", "A1B2", []string{"2#abc", "2#abc"}) {
				successes.Add(1)
			}
		})
	}
	wg.Wait()

	require.Equal(t, int32(1), successes.Load())
}

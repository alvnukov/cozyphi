package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/provider"
)

func TestQuotaSlotReleasedBeforePublication(t *testing.T) {
	ctrl := &Controller{}
	inFlightAtWake := make(chan bool, 1)
	ctrl.bus = NewBus(func() {
		ctrl.streamMu.Lock()
		inFlight := ctrl.usageWork.inFlight
		ctrl.streamMu.Unlock()
		inFlightAtWake <- inFlight
	})
	ctrl.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{}, nil
	})
	select {
	case inFlight := <-inFlightAtWake:
		assert.False(t, inFlight, "a receiver can immediately refresh for the new provider")
	case <-time.After(time.Second):
		t.Fatal("quota result was not published")
	}
}

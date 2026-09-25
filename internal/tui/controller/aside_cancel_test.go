package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

// stalledSSEServer sends the first words of an answer and then holds the
// stream open until the client goes away, which is what Esc looks like from
// the provider's end.
func stalledSSEServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"thinking it\"}}]}\n\n",
		)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Esc during a side question ends its row: the update that says it was
// cancelled still reaches the feed, since nothing else settles that row.
func TestEscDuringAnAsideStillSettlesItsRow(t *testing.T) {
	bus := NewBus(nil)
	ctrl := newInjectController(t, bus, stalledSSEServer(t).URL)
	t.Cleanup(ctrl.Close)
	seedRewindSession(t, ctrl)

	require.NoError(t, ctrl.Aside("what did we do?", ""))
	var updates []session.AsideUpdate
	waitForCond(t, 3*time.Second, func() bool {
		for _, m := range bus.Drain() {
			if ev, ok := m.(SessionEventMsg); ok {
				if update, ok := ev.Event.(session.AsideUpdate); ok {
					updates = append(updates, update)
				}
			}
		}
		return len(updates) > 0
	})
	require.Equal(t, session.StateStreaming, updates[0].State)

	ctrl.Cancel()
	updates = drainAside(t, bus)
	last := updates[len(updates)-1]
	assert.Equal(t, session.StateCancelled, last.State)
	waitForCond(t, 2*time.Second, func() bool { return !ctrl.RunActive() })
}

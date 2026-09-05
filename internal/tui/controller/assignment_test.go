package controller_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestAssignmentInterruptRetainsAssignmentUntilContinuation(t *testing.T) {
	for _, leave := range []bool{false, true} {
		t.Run(fmt.Sprintf("leave=%t", leave), func(t *testing.T) {
			entered := make(chan struct{})
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if requests.Add(1) == 1 {
					close(entered)
					<-r.Context().Done()
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(
					w,
					"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"continued answer\"}}]}\n\ndata: [DONE]\n\n",
				)
			}))
			defer server.Close()
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("COZYPHI_MODEL", "test-model")
			t.Setenv("COZYPHI_API_KEY", "test-key")
			t.Setenv("COZYPHI_BASE_URL", server.URL)
			cwd := t.TempDir()
			proj, err := project.Discover(cwd)
			require.NoError(t, err)
			ctrl, err := controller.NewController(controller.NewBus(nil), proj, cwd, "")
			require.NoError(t, err)
			defer ctrl.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			type result struct {
				text string
				err  error
			}
			finished := make(chan result, 1)
			go func() {
				text, err := ctrl.RunAssignment(ctx, "assignment-one", "first prompt")
				finished <- result{text, err}
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			ctrl.Cancel()
			require.Eventually(
				t,
				func() bool { return ctrl.Assignment().Turn == controller.TurnInterrupted },
				5*time.Second,
				time.Millisecond,
			)
			require.Equal(t, "assignment-one", ctrl.Assignment().JobID)
			select {
			case r := <-finished:
				t.Fatalf("interrupt ended assignment: %+v", r)
			default:
			}
			if leave {
				ctrl.LeaveAssignment()
			} else {
				ctrl.StartPrompt("continue", nil, "continuation")
				ctrl.LeaveAssignment()
			}
			select {
			case r := <-finished:
				if leave {
					require.ErrorIs(t, r.err, controller.ErrLeftInterrupted)
					require.EqualValues(t, 1, requests.Load())
				} else {
					require.NoError(t, r.err)
					require.Equal(t, "continued answer", r.text)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.True(t, ctrl.Assignment().Terminal)
		})
	}
}

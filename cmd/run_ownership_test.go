package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestHeadlessContinueSkipsActiveAndReleasesOwnership(t *testing.T) {
	for _, allBusy := range []bool{false, true} {
		t.Run(fmt.Sprintf("all_busy=%t", allBusy), func(t *testing.T) {
			proj, _ := testProject(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(
					w,
					"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"continued\"}}]}\n\ndata: [DONE]\n\n",
				)
			}))
			defer server.Close()
			t.Setenv("COZYPHI_MODEL", "fake")
			t.Setenv("COZYPHI_API_KEY", "test")
			t.Setenv("COZYPHI_BASE_URL", server.URL)
			bs, err := loadRunBootstrap(t.Context(), proj, t.TempDir(), false)
			require.NoError(t, err)
			makeHistory := func() *session.Manager {
				m, err := session.NewSessionManager(
					bs.Cwd,
					session.WithSessionDir(bs.SessionDir),
					session.WithShouldFlush(true),
				)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, m.Close()) })
				_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "seed"})
				require.NoError(t, err)
				return m
			}
			older, latest := makeHistory(), makeHistory()
			past := time.Now().Add(-time.Hour)
			require.NoError(t, os.Chtimes(older.File(), past, past))
			if !allBusy {
				require.NoError(t, older.Close())
			}
			before, err := os.ReadFile(latest.File())
			require.NoError(t, err)
			require.Equal(t, ExitOK, runHeadless(t.Context(), bs, runOptions{
				continueLast: true, prompt: "continue", timeout: 5 * time.Second,
			}))
			after, err := os.ReadFile(latest.File())
			require.NoError(t, err)
			require.Equal(t, before, after, "the latest active history must not be modified")
			list, err := session.ListSessions(bs.SessionDir)
			require.NoError(t, err)
			if allBusy {
				require.Len(t, list, 3, "no free history creates a fresh session")
				require.NotEqual(t, older.ID(), list[0].ID)
			} else {
				require.Len(t, list, 2)
				require.Equal(t, older.ID(), list[0].ID, "continue acquires the older free history")
			}
			require.False(t, list[0].Active, "headless exit releases its actual owner")
			reopened, err := session.OpenSession(list[0].File)
			require.NoError(t, err)
			require.NoError(t, reopened.Close())
		})
	}
}

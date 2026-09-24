package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPluginSessionStartContextReachesTheModelOnce(t *testing.T) {
	var (
		mu     sync.Mutex
		bodies []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	proj, cwd := pluginWorkspace(t, `{"hooks":{"SessionStart":[{"matcher":"startup|clear|compact",`+
		`"hooks":[{"type":"command","command":"echo PLUGIN-BOOT-SENTINEL"}]}]}}`)
	t.Setenv("COZYPHI_MODEL", "fake")
	t.Setenv("COZYPHI_API_KEY", "x")
	t.Setenv("COZYPHI_BASE_URL", server.URL)
	require.NoError(t, proj.LoadConfig())

	c, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(c.Close)

	requests := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
	for i, prompt := range []string{"first", "second"} {
		c.StartPrompt(prompt, nil)
		require.Eventually(t, func() bool { return len(requests()) == i+1 && !c.RunActive() },
			5*time.Second, 10*time.Millisecond)
	}
	got := requests()
	require.Equal(t, 1, strings.Count(got[0], "PLUGIN-BOOT-SENTINEL"))
	require.Equal(t, 1, strings.Count(got[1], "PLUGIN-BOOT-SENTINEL"), "delivered once, then only history")
}

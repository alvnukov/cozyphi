package agent

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// TestSettersDuringLoopAreRaceFree pins the mid-run reconfiguration contract:
// a running round works off an immutable roundSnapshot while the TUI
// goroutine swaps mode, permission, hooks and model under the engine lock.
// Run with -race; before the snapshot+lock this tripped the detector on
// engine.client and engine.executor.
func TestSettersDuringLoopAreRaceFree(t *testing.T) {
	const toolRounds = 40
	server, _ := recordingServer(t, func(request int, w http.ResponseWriter) {
		time.Sleep(3 * time.Millisecond) // keep rounds in flight while setters hammer
		if request <= toolRounds {
			_, _ = fmt.Fprint(w, sseToolCallChunk(
				fmt.Sprintf("call_%d", request), "read", `{"path":"."}`,
			))
			return
		}
		_, _ = fmt.Fprint(w, sseTextChunk())
	})

	engine, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
	})
	require.NoError(t, err)

	done := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		var lastErr error
		for _, err := range engine.Loop(t.Context(), "keep running", LoopOpts{}) {
			if err != nil {
				lastErr = err
			}
		}
		errCh <- lastErr
		close(done)
	}()

	hammer := time.NewTicker(300 * time.Microsecond)
	defer hammer.Stop()
	deadline := time.After(500 * time.Millisecond)
	for i := 0; ; i++ {
		select {
		case <-deadline:
			<-done
			select {
			case err := <-errCh:
				require.NoError(t, err)
			default:
			}
			return
		case <-hammer.C:
		}
		switch i % 4 {
		case 0:
			engine.SetMode(ModeBuild)
		case 1:
			engine.SetMode(ModeUsePlan)
		case 2:
			engine.SetPermission(engine.gate, engine.ask)
		case 3:
			require.NoError(t, engine.SetModel(llm.ModelConfig{
				Name: "fake", BaseURL: server.URL, APIKey: "x",
			}))
		}
		_ = engine.Mode()
		_ = engine.HasTool("read")
		_ = engine.ToolNames()
	}
}

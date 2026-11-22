package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/util"
)

func TestCodeModeSelectionForBothEntryPoints(t *testing.T) {
	for _, flag := range []string{"--code-mode", "--direct-tools"} {
		tui, err := parseTUIArgs([]string{flag})
		require.NoError(t, err)
		run, err := parseRunArgs([]string{flag, "-p", "task"})
		require.NoError(t, err)
		for _, configured := range []bool{true, false} {
			cfg := &project.Config{CodeMode: project.CodeModeConfig{Enabled: configured}}
			require.Equal(t, configured, codeModeEnabled(cfg, ""))
			require.Equal(t, flag == "--code-mode", codeModeEnabled(cfg, tui.toolSurface))
			require.Equal(t, flag == "--code-mode", codeModeEnabled(cfg, run.toolSurface))
		}
	}
}

type cellModelTransport func(*http.Request) (*http.Response, error)

func (f cellModelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHeadlessCodeModeExecutesFirstCell(t *testing.T) {
	_, pathDir := testProject(t)
	for _, name := range []string{"fd", "rg"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(pathDir, name), []byte("fixture"), 0o700))
	}
	for _, name := range []string{"COZYPHI_MODEL", "COZYPHI_BASE_URL", "COZYPHI_API_KEY"} {
		t.Setenv(name, "")
	}
	p, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	cfg := "models:\n  - name: fixture\n    protocol: openai\n    base_url: https://fixture.invalid/v1\n    api_key: fixture\ncode_mode:\n  enabled: true\n  discovery: manual\nagents:\n  enabled: false\nopencode:\n  enabled: false\n"
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(cfg), 0o600))
	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)
	client := util.DefaultHTTPClient()
	prior := client.Transport
	t.Cleanup(func() { client.Transport = prior })
	var requests int
	client.Transport = cellModelTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "fixture.invalid" {
			return nil, fmt.Errorf("unexpected provider %q", r.URL.Hostname())
		}
		var body struct {
			Tools    []struct{ Function struct{ Name string } }
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		requests++
		delta := map[string]any{"role": "assistant", "content": "verified"}
		if requests == 1 {
			var names []string
			for _, tool := range body.Tools {
				names = append(names, tool.Function.Name)
			}
			if !slices.Contains(names, "code") || slices.Contains(names, "read") {
				return nil, fmt.Errorf("unexpected first-request tools: %v", names)
			}
			delta = headlessToolDelta(
				"first-cell",
				"code",
				`{"source":"return {answer:6*7,readable:tools.list().includes('read')}"}`,
			)
		} else {
			if len(body.Messages) == 0 {
				return nil, fmt.Errorf("missing tool response on request %d", requests)
			}
			last := body.Messages[len(body.Messages)-1]
			if last.Role != "tool" || !strings.Contains(last.Content, `"answer":42`) ||
				!strings.Contains(last.Content, `"readable":true`) {
				return nil, fmt.Errorf("expected cell result on request %d, got %q", requests, last.Content)
			}
		}
		event, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Request:    r,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("data: " + string(event) + "\n\ndata: [DONE]\n\n")),
		}, nil
	})
	status := runHeadless(
		t.Context(),
		bs,
		runOptions{prompt: "compute six times seven", maxRounds: 3, timeout: 45 * time.Second},
	)
	require.Equal(t, ExitOK, status)
	require.Equal(t, 2, requests)
}

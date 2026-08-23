package contexttool_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/tools/contexttool"
)

func testDeps(stats contexttool.Stats, compactErr error) (contexttool.Deps, *int) {
	requests := 0
	return contexttool.Deps{
		Stats: func() contexttool.Stats { return stats },
		RequestCompact: func() error {
			requests++
			return compactErr
		},
	}, &requests
}

func TestContextToolStatusReportsQuantitativeUsage(t *testing.T) {
	deps, _ := testDeps(contexttool.Stats{
		ContextTokens:   9000,
		TokenSource:     "provider",
		UsedBytes:       40960,
		Messages:        12,
		ContextWindow:   131072,
		ThresholdTokens: 131072 - 16384,
	}, nil)
	deps.RequestCompact = func() error {
		t.Fatal("status must not request compaction")
		return nil
	}

	tool := contexttool.Tools(deps)
	require.Equal(t, "context", tool.Definition.Name)

	for _, input := range []json.RawMessage{nil, []byte(`{}`), []byte(`{"action":"status"}`)} {
		res, err := tool.Run(t.Context(), input)
		require.NoError(t, err)

		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.Content), &payload))
		require.EqualValues(t, 9000, payload["context_tokens"])
		require.Equal(t, "provider", payload["token_source"])
		require.EqualValues(t, 40.0, payload["context_kb"])
		require.EqualValues(t, 12, payload["messages"])
		require.EqualValues(t, 131072, payload["context_window"])
		require.EqualValues(t, 131072-16384, payload["compact_threshold_tokens"])
		require.Equal(t, false, payload["compaction_recommended"])
		require.Contains(t, res.Content, "below the compact threshold")
	}
}

func TestContextToolStatusRecommendsCompaction(t *testing.T) {
	deps, _ := testDeps(contexttool.Stats{
		ContextTokens:         120000,
		TokenSource:           "provider",
		UsedBytes:             500000,
		Messages:              40,
		ContextWindow:         131072,
		ThresholdTokens:       114688,
		CompactionRecommended: true,
	}, nil)

	res, err := contexttool.Tools(deps).Run(t.Context(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, res.Content, `"compaction_recommended":true`)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Content), &payload))
	require.Contains(t, payload["note"], `"action":"compact"`)
}

func TestContextToolStatusUnknownWindow(t *testing.T) {
	deps, _ := testDeps(contexttool.Stats{
		ContextTokens: 5000,
		TokenSource:   "estimate",
		UsedBytes:     20000,
		Messages:      4,
	}, nil)

	res, err := contexttool.Tools(deps).Run(t.Context(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, res.Content, "context window size unknown")
}

func TestContextToolCompactSchedules(t *testing.T) {
	deps, requests := testDeps(contexttool.Stats{ContextTokens: 1}, nil)

	res, err := contexttool.Tools(deps).Run(t.Context(), json.RawMessage(`{"action":"compact"}`))
	require.NoError(t, err)
	require.Equal(t, 1, *requests)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Content), &payload))
	require.Equal(t, "scheduled", payload["status"])
	require.Contains(t, payload["note"], "tool round")
}

func TestContextToolCompactErrorPropagates(t *testing.T) {
	sentinel := errors.New("nothing to compact")
	deps, requests := testDeps(contexttool.Stats{}, sentinel)

	_, err := contexttool.Tools(deps).Run(t.Context(), json.RawMessage(`{"action":"compact"}`))
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, 1, *requests)
}

func TestContextToolRejectsUnknownAction(t *testing.T) {
	deps, _ := testDeps(contexttool.Stats{}, nil)

	_, err := contexttool.Tools(deps).Run(t.Context(), json.RawMessage(`{"action":"purge"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "purge")
}

func TestContextToolDetailFromArgs(t *testing.T) {
	deps, _ := testDeps(contexttool.Stats{}, nil)
	tool := contexttool.Tools(deps)

	require.Equal(t, "compact", tool.DetailFromArgs(json.RawMessage(`{"action":"compact"}`)))
	require.Equal(t, "status", tool.DetailFromArgs(json.RawMessage(`{}`)))
	require.Equal(t, "status", tool.DetailFromArgs(nil))
}

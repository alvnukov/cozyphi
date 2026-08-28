package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const matchingToolsBody = `{"jsonrpc":"2.0","id":7,"result":{"tools":[{"name":"echo"}]}}`

func TestParseHTTPOrSSEBodySelectsResponseByID(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "progress notification precedes the matching response",
			body: "event: message\n" +
				"data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n" +
				"data: " + matchingToolsBody + "\n\n",
		},
		{
			name: "foreign-id response precedes the matching response",
			body: "data: {\"jsonrpc\":\"2.0\",\"id\":9999,\"result\":{\"tools\":[]}}\n\n" +
				"data: " + matchingToolsBody + "\n\n",
		},
		{
			name: "server-to-client request reusing our id is not the answer",
			body: "data: {\"jsonrpc\":\"2.0\",\"id\":7,\"method\":\"roots/list\",\"params\":{}}\n\n" +
				"data: " + matchingToolsBody + "\n\n",
		},
		{
			name: "data prefix without a space still carries the payload",
			body: "data:" + matchingToolsBody + "\n\n",
		},
		{
			name: "plain json body",
			body: matchingToolsBody + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rpc, err := parseHTTPOrSSEBody([]byte(tt.body), 7)
			require.NoError(t, err)
			tools, err := decodeToolsList(rpc.Result)
			require.NoError(t, err)
			require.Len(t, tools, 1)
			assert.Equal(t, "echo", tools[0].Name)
		})
	}
}

func TestParseHTTPOrSSEBodyFailsClosedWithoutMatch(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "sse body never answers our id",
			body: "data: {\"jsonrpc\":\"2.0\",\"id\":9999,\"result\":{\"tools\":[]}}\n\n",
		},
		{
			name: "notification only",
			body: "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n",
		},
		{
			name: "plain json with a foreign id",
			body: `{"jsonrpc":"2.0","id":9999,"result":{}}`,
		},
		{
			name: "garbage",
			body: "<html>oops</html>",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rpc, err := parseHTTPOrSSEBody([]byte(tt.body), 7)
			require.ErrorIs(t, err, errTransportDead, "an unmatchable body must drop the handshake")
			assert.Contains(t, err.Error(), "id 7")
			assert.Empty(t, rpc.Result)
		})
	}
}

// The transport must thread its request id into body parsing and propagate the
// dead-transport sentinel: the session's re-handshake recovery hangs off it.
func TestHTTPTransportPicksMatchingResponseFromSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		notif, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]any{},
		})
		foreign, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": 9999,
			"result": map[string]any{"tools": []map[string]string{{"name": "foreign"}}},
		})
		correct, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"result": map[string]any{"tools": []map[string]string{{"name": "echo"}}},
		})
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + string(notif) + "\n\n" +
			"data: " + string(foreign) + "\n\n" +
			"data: " + string(correct) + "\n\n"))
	}))
	defer srv.Close()

	tr, err := newHTTPTransport("sse", ServerConfig{Transport: "http", URL: srv.URL})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.close() })

	raw, err := tr.call(t.Context(), "tools/list", nil)
	require.NoError(t, err)
	tools, err := decodeToolsList(raw)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "echo", tools[0].Name)
}

func TestHTTPTransportUnmatchableBodyIsDeadTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"id\":9999,\"result\":{}}\n\n"))
	}))
	defer srv.Close()

	tr, err := newHTTPTransport("sse", ServerConfig{Transport: "http", URL: srv.URL})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.close() })

	_, err = tr.call(t.Context(), "tools/list", nil)
	require.ErrorIs(t, err, errTransportDead)
}

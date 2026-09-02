package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fake stdio server re-executes the test binary with
// COZYPHI_TEST_MCP_SERVER=1 and answers JSON-RPC from stdin. Tool names:
//
//	echo    — answers immediately with "echo:<message>"
//	hang    — never answers (the process stays alive reading stdin)
//	wrongid — writes a foreign-id response and a notification, then answers
//	srvid   — writes a server-to-client request reusing our id, then answers
func TestMain(m *testing.M) {
	if os.Getenv("COZYPHI_TEST_MCP_SERVER") == "1" {
		fakeStdioServerMain()
		return
	}
	os.Exit(m.Run())
}

func fakeStdioServerMain() {
	respond := func(id float64, result any) {
		payload, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result":  result,
		})
		fmt.Println(string(payload))
	}
	textResult := func(id float64, text string) {
		respond(id, map[string]any{"content": []map[string]string{{"type": "text", "text": text}}})
	}

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req struct {
			ID     *float64 `json:"id"`
			Method string   `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil || req.Method == "" {
			continue // notification or garbage: ignore
		}
		switch req.Method {
		case "initialize":
			respond(*req.ID, map[string]any{
				"serverInfo": map[string]string{"name": "fake", "version": "0"},
			})
		case "tools/list":
			respond(*req.ID, map[string]any{"tools": []map[string]any{
				{"name": "echo", "description": "echo"},
				{"name": "hang", "description": "hang"},
				{"name": "bigframe", "description": "bigframe"},
				{"name": "deepnest", "description": "deepnest"},
				{"name": "notiflood", "description": "notiflood"},
				{"name": "wrongid", "description": "wrongid"},
				{"name": "srvid", "description": "srvid"},
			}})
		case "tools/call":
			switch req.Params.Name {
			case "echo":
				textResult(*req.ID, "echo:"+fmt.Sprint(req.Params.Arguments["message"]))
			case "hang":
				// no response; keep reading so the process stays alive
			case "bigframe":
				// 2 MiB with no newline: must trip the frame limit, not grow the reader
				_, _ = os.Stdout.WriteString(strings.Repeat("x", 2*maxFrameBytes))
			case "deepnest":
				// one bounded line nested far past encoding/json's depth limit
				fmt.Println(`{"result":` + strings.Repeat("[", 200000))
			case "notiflood":
				notif, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/progress",
					"params":  map[string]any{},
				})
				for range 5000 {
					fmt.Println(string(notif))
				}
				textResult(*req.ID, "correct")
			case "wrongid":
				respond(9999, map[string]any{
					"content": []map[string]string{{"type": "text", "text": "foreign"}},
				})
				notif, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/progress",
					"params":  map[string]any{},
				})
				fmt.Println(string(notif))
				textResult(*req.ID, "correct")
			case "srvid":
				serverRequest, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0",
					"id":      *req.ID,
					"method":  "roots/list",
					"params":  map[string]any{},
				})
				fmt.Println(string(serverRequest))
				textResult(*req.ID, "correct")
			}
		}
	}
}

func newFakeStdioSession(t *testing.T, timeout string) *session {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	tr, err := newStdioTransport("fake", ServerConfig{
		Command: []string{exe},
		Env:     map[string]string{"COZYPHI_TEST_MCP_SERVER": "1"},
		Timeout: timeout,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.close() })
	return newSession("fake", tr)
}

func TestStdioCallMatchesResponseByID(t *testing.T) {
	sess := newFakeStdioSession(t, "10s")
	require.NoError(t, sess.Initialize(t.Context()))

	got, err := sess.CallTool(t.Context(), "wrongid", map[string]any{"message": "hi"})
	require.NoError(t, err)
	assert.Equal(t, "correct", got, "foreign-id response and notification must be skipped")
}

func TestStdioSkipsServerToClientRequestsWithOurID(t *testing.T) {
	sess := newFakeStdioSession(t, "10s")
	require.NoError(t, sess.Initialize(t.Context()))

	got, err := sess.CallTool(t.Context(), "srvid", nil)
	require.NoError(t, err)
	assert.Equal(t, "correct", got, "a server-to-client request reusing our id is not the answer")
}

func TestStdioTimeoutClosesSessionThenNextCallSucceeds(t *testing.T) {
	sess := newFakeStdioSession(t, "250ms")
	require.NoError(t, sess.Initialize(t.Context()))

	_, err := sess.CallTool(t.Context(), "hang", nil)
	require.ErrorIs(t, err, errTransportDead)
	assert.Contains(t, err.Error(), "timeout after")

	// The timeout closed the transport; the session must re-handshake over a
	// fresh process instead of feeding the next call a stale or dead pipe.
	got, err := sess.CallTool(t.Context(), "echo", map[string]any{"message": "again"})
	require.NoError(t, err)
	assert.Equal(t, "echo:again", got)
}

// fakeDeadTransport pins the session-level contract: an errTransportDead from
// any transport drops the handshake state so the next call re-initializes.
type fakeDeadTransport struct {
	mu         sync.Mutex
	initCalls  int
	failNext   bool
	closeCalls int
}

func (f *fakeDeadTransport) call(_ context.Context, method string, _ map[string]any) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return nil, fmt.Errorf("mcp %s: wire gone (%w)", method, errTransportDead)
	}
	switch method {
	case "initialize":
		f.initCalls++
		return json.RawMessage(`{}`), nil
	case "tools/list":
		return json.RawMessage(`{"tools":[{"name":"echo"}]}`), nil
	}
	return json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`), nil
}

func (f *fakeDeadTransport) notify(context.Context, string, map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return nil
}

func (f *fakeDeadTransport) close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeCalls++
	return nil
}

func TestSessionResetsHandshakeOnDeadTransport(t *testing.T) {
	tr := &fakeDeadTransport{}
	sess := newSession("fake", tr)

	require.NoError(t, sess.Initialize(t.Context()))
	tools, err := sess.ListTools(t.Context())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, 1, tr.initCalls)

	tr.failNext = true
	_, err = sess.CallTool(t.Context(), "echo", nil)
	require.ErrorIs(t, err, errTransportDead)
	assert.Equal(t, 1, tr.closeCalls, "dead transport must be closed")

	// The tool cache died with the wire: listing again must re-initialize
	// and re-fetch instead of serving the pre-death copy.
	tools, err = sess.ListTools(t.Context())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, 2, tr.initCalls, "session must re-initialize after wire death")
}

func TestResponseIDMatches(t *testing.T) {
	assert.True(t, responseIDMatches(float64(7), 7))
	assert.True(t, responseIDMatches("7", 7))
	assert.False(t, responseIDMatches(float64(8), 7))
	assert.False(t, responseIDMatches("x", 7))
	assert.False(t, responseIDMatches(nil, 7))
	assert.False(t, responseIDMatches(true, 7))
}

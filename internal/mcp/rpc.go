package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/alvnukov/cozyphi/internal/util"
)

// jsonRPCError is the JSON-RPC 2.0 error object.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	if e == nil {
		return "mcp: nil error"
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// jsonRPCResponse is a JSON-RPC 2.0 response (id optional for parsing).
type jsonRPCResponse struct {
	ID     any             `json:"id"`
	Method string          `json:"method"` // set on notifications / server requests
	Result json.RawMessage `json:"result"`
	Error  *jsonRPCError   `json:"error"`
}

func marshalRequest(id int64, method string, params map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
}

func marshalNotification(method string, params map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func nextID(counter *atomic.Int64) int64 {
	return counter.Add(1)
}

// responseIDMatches reports whether a decoded response id identifies the given
// request. JSON numbers decode as float64; string ids are accepted for servers
// that echo them quoted. Anything else (nil, bool, objects) is not our answer.
func responseIDMatches(got any, want int64) bool {
	switch v := got.(type) {
	case float64:
		return v == float64(want)
	case string:
		return v == strconv.FormatInt(want, 10)
	default:
		return false
	}
}

func decodeToolsList(raw json.RawMessage) ([]ToolDef, error) {
	var parsed struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return parsed.Tools, nil
}

// extractToolContent turns a tools/call result into model-facing text.
func extractToolContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return string(raw)
	}
	var b strings.Builder
	for _, part := range result.Content {
		if part.Type == "text" || part.Type == "" {
			b.WriteString(part.Text)
		}
	}
	out := b.String()
	if out == "" {
		return string(raw)
	}
	if result.IsError {
		return "error: " + out
	}
	return out
}

// parseHTTPOrSSEBody selects the JSON-RPC response for id from a plain-JSON or
// SSE body. SSE bodies interleave notifications and responses to other
// requests; only the id pair — on a message with no method field — pins the
// answer. A body that never yields our response fails closed as a dead
// transport: pairing with the server cannot be trusted, so the session drops
// its handshake and re-initializes on the next call.
func parseHTTPOrSSEBody(body []byte, id int64) (jsonRPCResponse, error) {
	for data, err := range util.ParseDataStream(bytes.NewReader(body)) {
		if err != nil {
			break // in-memory body; a scan error is impossible before the cap
		}
		var rpc jsonRPCResponse
		if json.Unmarshal(data, &rpc) != nil || rpc.Method != "" {
			continue // notification, server request, or not JSON-RPC; keep scanning
		}
		if responseIDMatches(rpc.ID, id) {
			return rpc, nil
		}
	}
	// No data line answered us; the body may still be one plain JSON response.
	var rpc jsonRPCResponse
	if json.Unmarshal(body, &rpc) == nil && rpc.Method == "" && responseIDMatches(rpc.ID, id) {
		return rpc, nil
	}
	return jsonRPCResponse{}, fmt.Errorf(
		"no response for id %d (%w); raw=%q", id, errTransportDead, truncate(string(body), 200),
	)
}

func resultOrError(method string, rpc jsonRPCResponse) (json.RawMessage, error) {
	if rpc.Error != nil {
		return nil, fmt.Errorf("mcp %s: %w", method, rpc.Error)
	}
	return rpc.Result, nil
}

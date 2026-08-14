package mcp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pulseaiclub/phi/internal/mcp"
)

func TestHTTPClientJSONAndSSE(t *testing.T) {
	var (
		mu        sync.Mutex
		session   string
		sawHeader bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &req)

		mu.Lock()
		if r.Header.Get("Authorization") == "Bearer test" {
			sawHeader = true
		}
		if session == "" {
			session = "sess-1"
		}
		sid := session
		mu.Unlock()
		w.Header().Set("Mcp-Session-Id", sid)

		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]string{"name": "http-echo", "version": "0.1"},
			}
			writeJSONRPC(w, req.ID, result)
		case "tools/list":
			// SSE style like mcptoon
			result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{"type": "string"},
							},
							"required": []string{"message"},
						},
					},
				},
			}
			payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: message\ndata: " + string(payload) + "\n\n"))
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			msg, _ := p.Arguments["message"].(string)
			text, _ := json.Marshal(map[string]string{"echo": msg})
			result = map[string]any{
				"content": []map[string]string{{"type": "text", "text": string(text)}},
			}
			writeJSONRPC(w, req.ID, result)
		default:
			http.Error(w, "unknown", 400)
		}
	}))
	defer srv.Close()

	c, err := mcp.NewClient("httpdemo", mcp.ServerConfig{
		Transport: "http",
		URL:       srv.URL,
		Headers:   map[string]string{"Authorization": "Bearer test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}
	out, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("out = %q", out)
	}
	mu.Lock()
	okHeader := sawHeader
	mu.Unlock()
	if !okHeader {
		t.Fatal("expected Authorization header")
	}
}

func writeJSONRPC(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func TestNewClientUnsupportedTransport(t *testing.T) {
	_, err := mcp.NewClient("x", mcp.ServerConfig{Transport: "grpc", Command: []string{"true"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("got %v", err)
	}
}

func TestNewClientHTTPRequiresURL(t *testing.T) {
	_, err := mcp.NewClient("x", mcp.ServerConfig{Transport: "http"})
	if err == nil || !strings.Contains(err.Error(), "url") {
		t.Fatalf("got %v", err)
	}
}

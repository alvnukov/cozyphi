package main

import (
	"bufio"
	"encoding/json"
	"os"
)

// Minimal MCP stdio echo server for phi tests.
func main() {
	sc := bufio.NewScanner(os.Stdin)
	// allow large JSON-RPC lines
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      any             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.Method == "notifications/initialized" || req.ID == nil && req.Method != "" {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "echo", "version": "0.1.0"},
			}
		case "tools/list":
			result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "Echo back the input message",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"message": map[string]any{"type": "string", "description": "Message to echo"},
							},
							"required": []string{"message"},
						},
					},
					{
						"name":        "add",
						"description": "Add two numbers",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"a": map[string]any{"type": "number"},
								"b": map[string]any{"type": "number"},
							},
							"required": []string{"a", "b"},
						},
					},
				},
			}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			text := ""
			switch p.Name {
			case "echo":
				msg, _ := p.Arguments["message"].(string)
				b, _ := json.Marshal(map[string]string{"echo": msg})
				text = string(b)
			case "add":
				a, _ := p.Arguments["a"].(float64)
				b, _ := p.Arguments["b"].(float64)
				raw, _ := json.Marshal(map[string]float64{"sum": a + b})
				text = string(raw)
			default:
				text = `{"error":"unknown tool"}`
			}
			result = map[string]any{
				"content": []map[string]string{{"type": "text", "text": text}},
			}
		default:
			resp, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error":   map[string]any{"code": -32601, "message": "Unknown method: " + req.Method},
			})
			os.Stdout.Write(append(resp, '\n'))
			continue
		}
		resp, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
		os.Stdout.Write(append(resp, '\n'))
	}
}

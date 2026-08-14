package mcptool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
)

// Tools returns the three MCP meta-tools bound to pool.
// If pool is nil, returns nil.
func Tools(pool *mcp.Pool) []tooldef.Tool {
	if pool == nil {
		return nil
	}
	return []tooldef.Tool{
		listTool(pool),
		inspectTool(pool),
		callTool(pool),
	}
}

func listTool(pool *mcp.Pool) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "mcp_list",
			Description: `List MCP servers or tool names on one server (compact text, not full JSON schemas).

Without server: space-separated configured server names.
With server: space-separated tool names on that server.
Schemas never enter the model context — use mcp_inspect for one tool's params.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"server": llm.Object{
						"type":        "string",
						"description": "Optional MCP server name. Omit to list servers.",
					},
				},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Server string `json:"server"`
			}
			_ = json.Unmarshal(input, &in)
			if in.Server == "" {
				return "servers"
			}
			return in.Server
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Server string `json:"server"`
			}
			if len(input) > 0 && string(input) != "null" {
				if err := json.Unmarshal(input, &in); err != nil {
					return tooldef.Result{}, fmt.Errorf("mcp_list: %w", err)
				}
			}
			if in.Server == "" {
				names := pool.ServerNames()
				body := mcp.CompactServerList(names)
				return tooldef.Result{Content: body, Detail: fmt.Sprintf("%d servers", len(names)), Output: body}, nil
			}
			tools, err := pool.ListTools(ctx, in.Server)
			if err != nil {
				return tooldef.Result{}, err
			}
			body := mcp.CompactToolNames(tools)
			return tooldef.Result{
				Content: body,
				Detail:  fmt.Sprintf("%s: %d tools", in.Server, len(tools)),
				Output:  body,
			}, nil
		},
	}
}

func inspectTool(pool *mcp.Pool) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "mcp_inspect",
			Description: `Show a compact parameter summary for one MCP tool (slim text).

Use after mcp_list to learn required args before mcp_call.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"server": llm.Object{
						"type":        "string",
						"description": "MCP server name",
					},
					"tool": llm.Object{
						"type":        "string",
						"description": "Tool name on that server",
					},
				},
				Required: []string{"server", "tool"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Server string `json:"server"`
				Tool   string `json:"tool"`
			}
			_ = json.Unmarshal(input, &in)
			return in.Server + "/" + in.Tool
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Server string `json:"server"`
				Tool   string `json:"tool"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("mcp_inspect: %w", err)
			}
			def, err := pool.Inspect(ctx, in.Server, in.Tool)
			if err != nil {
				return tooldef.Result{}, err
			}
			body := mcp.SlimTool(*def)
			return tooldef.Result{Content: body, Detail: in.Server + "/" + in.Tool, Output: body}, nil
		},
	}
}

func callTool(pool *mcp.Pool) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "mcp_call",
			Description: `Call one MCP tool on a configured server.

Prefer mcp_list then mcp_inspect before calling unfamiliar tools.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"server": llm.Object{
						"type":        "string",
						"description": "MCP server name",
					},
					"tool": llm.Object{
						"type":        "string",
						"description": "Tool name on that server",
					},
					"args": llm.Object{
						"type":        "object",
						"description": "JSON object of tool arguments",
					},
				},
				Required: []string{"server", "tool"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Server string `json:"server"`
				Tool   string `json:"tool"`
			}
			_ = json.Unmarshal(input, &in)
			return in.Server + "/" + in.Tool
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Server string         `json:"server"`
				Tool   string         `json:"tool"`
				Args   map[string]any `json:"args"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("mcp_call: %w", err)
			}
			out, err := pool.Call(ctx, in.Server, in.Tool, in.Args)
			if err != nil {
				return tooldef.Result{}, err
			}
			body := mcp.FormatCallResult(out, 32_000)
			return tooldef.Result{Content: body, Detail: in.Server + "/" + in.Tool, Output: body}, nil
		},
	}
}

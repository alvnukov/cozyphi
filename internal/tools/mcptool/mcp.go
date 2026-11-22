package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
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
			Description: `List MCP tool names on one server (compact text, not full JSON schemas).

Returns space-separated tool names. Schemas never enter the model context — use mcp_inspect for one tool's params.`,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"server": llm.Object{
						"type":        "string",
						"description": "MCP server name",
					},
				},
				Required: []string{"server"},
			},
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in struct {
				Server string `json:"server"`
			}
			_ = json.Unmarshal(input, &in)
			return in.Server
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Server string `json:"server"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("mcp_list: %w", err)
			}
			if in.Server == "" {
				return tooldef.Result{}, errors.New("mcp_list: server is required")
			}
			tools, err := pool.ListTools(ctx, in.Server)
			if err != nil {
				return tooldef.Result{}, err
			}
			names := make([]string, 0, len(tools))
			for _, tool := range tools {
				names = append(names, tool.Name)
			}
			body := mcp.CompactToolNames(tools)
			return tooldef.Result{
				Content: body, Structured: names,
				Detail: fmt.Sprintf("%s: %d tools", in.Server, len(tools)),
				Output: body,
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
			return in.Server + " · " + in.Tool
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
			return tooldef.Result{Content: body, Structured: def, Detail: in.Server + "/" + in.Tool, Output: body}, nil
		},
	}
}

func callTool(pool *mcp.Pool) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name: "mcp_call",
			Description: `Call one MCP tool on a configured server.

Prefer mcp_list then mcp_inspect before calling unfamiliar tools.
Code Mode returns content blocks, structuredContent when supplied, and isError. To read a returned binary artifact, use the same server and tool and pass artifact {id, offset?, limit?} instead of args. Offsets count decoded bytes. A large result returns resultArtifact. Concatenate its UTF-8 text pages using nextOffset and JSON.parse the complete string.`,
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
					"artifact": llm.Object{
						"type":        "object",
						"description": "Read stored media or a JSON result from this tool without running it again.",
						"properties": llm.Object{
							"id":     llm.Object{"type": "string"},
							"offset": llm.Object{"type": "integer", "minimum": 0},
							"limit":  llm.Object{"type": "integer", "minimum": 1, "maximum": 131072},
						},
						"required": []string{"id"},
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
			return in.Server + " · " + in.Tool
		},
		Run: func(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
			var in struct {
				Server   string         `json:"server"`
				Tool     string         `json:"tool"`
				Args     map[string]any `json:"args"`
				Artifact *struct {
					ID     string `json:"id"`
					Offset int    `json:"offset"`
					Limit  int    `json:"limit"`
				} `json:"artifact"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tooldef.Result{}, fmt.Errorf("mcp_call: %w", err)
			}
			if page := in.Artifact; page != nil {
				if in.Args != nil || page.ID == "" {
					return tooldef.Result{}, errors.New("mcp_call: artifact requires an id and excludes args")
				}
				data, err := pool.ReadArtifact(in.Server, in.Tool, page.ID, page.Offset, page.Limit)
				if err != nil {
					return tooldef.Result{}, err
				}
				body, err := json.Marshal(data)
				if err != nil {
					return tooldef.Result{}, fmt.Errorf("encode artifact page: %w", err)
				}
				text := string(body)
				if len(body) > 32_000 {
					text = "Artifact page exceeds the direct display limit. Request a smaller limit (for example, 4096 bytes)."
				}
				return tooldef.Result{
					Content:    text,
					Structured: data,
					Detail:     in.Server + "/" + in.Tool + " artifact",
				}, nil
			}
			out, err := pool.CallResult(ctx, in.Server, in.Tool, in.Args)
			if err != nil {
				return tooldef.Result{}, err
			}
			body := mcp.FormatCallResult(out.Text(), 32_000)
			program, err := pool.ProgramResult(in.Server, in.Tool, out)
			if err != nil {
				return tooldef.Result{}, err
			}
			return tooldef.Result{
				Content:    body,
				Structured: program,
				Detail:     in.Server + "/" + in.Tool,
				Output:     body,
			}, nil
		},
	}
}

// Package codehost binds the program runtime, shared library and discovery to
// the per-session dispatcher used by both the terminal and headless commands.
package codehost

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/codemode"
	"github.com/alvnukov/cozyphi/internal/debuglog"
	"github.com/alvnukov/cozyphi/internal/funcadvisor"
	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/librarytool"
)

// New binds an explicitly enabled process. No global store is created until a
// session writes a draft or pins its first release.
func New(layout project.GlobalLayout, cfg project.CodeModeConfig, models []llm.ModelConfig) *agent.CodeMode {
	var advisor *llm.ModelConfig
	if cfg.AdvisorModel != "" {
		for i := range models {
			if models[i].Name == cfg.AdvisorModel {
				advisor = &models[i]
				break
			}
		}
		if advisor == nil {
			debuglog.Logf("code mode: configured advisor is unavailable; using local discovery")
		}
	}
	store := funclib.New(layout.FunctionsDir())
	if cfg.Discovery == "local" {
		advisor = nil
	}
	selector := funcadvisor.NewWithPolicy(store, advisor, cfg.AdvisorTimeout, funcadvisor.ModelPolicy(cfg.Discovery))
	var checkouts sync.Map
	checkout := func(cwd string) string {
		if root, ok := checkouts.Load(cwd); ok {
			return root.(string)
		}
		root := project.CheckoutFor(cwd)
		checkouts.Store(cwd, root)
		return root
	}
	mode := &agent.CodeMode{
		Tools: func(sessionID, cwd string, writable bool, definitions []llm.ToolDefinition) []tools.Tool {
			root := checkout(cwd)
			return []tools.Tool{
				librarytool.Tool(store, sessionID, root, writable, definitions, nil),
				cellTestTool(store, root, definitions),
			}
		},
		Hints: func(ctx context.Context, sessionID, cwd, task string, definitions []llm.ToolDefinition) string {
			return selector.Hints(ctx, sessionID, checkout(cwd), task, definitions)
		},
	}
	mode.Run = func(ctx context.Context, _, cwd string, writable bool, source string, definitions []llm.ToolDefinition,
		dispatch func(context.Context, string, json.RawMessage) (any, error),
	) (agent.CodeResult, error) {
		resolve := store.PreviewPinned
		if writable {
			resolve = store.ResolvePinned
		}
		modules, pins, err := resolve(ctx, checkout(cwd), codemode.Imports(source))
		var result agent.CodeResult
		if err != nil {
			return result, fmt.Errorf("resolve cell imports: %w", err)
		}
		if len(pins) != 0 {
			result.Imports = make(map[string]string, len(pins))
			for name, pin := range pins {
				result.Imports[name] = pin.Version + "@" + pin.Digest
			}
		}
		out, err := codemode.Run(
			ctx,
			codemode.Program{Source: source, Modules: modules, Tools: definitions, ParallelReads: true},
			func(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
				value, callErr := dispatch(ctx, name, args)
				if callErr != nil {
					return nil, callErr
				}
				return json.Marshal(value)
			},
		)
		result.Output = string(out)
		return result, err
	}
	if cfg.Discovery == "manual" {
		mode.Hints = nil
	}
	return mode
}

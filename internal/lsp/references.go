package lsp

import (
	"context"
	"encoding/json"
	"fmt"
)

// references implements textDocument/references with the frozen
// includeDeclaration context and deduplicated, workspace-contained
// locations.
func (m *Manager) references(ctx context.Context, c *client, q Query) (Result, error) {
	if err := c.requireCapability("referencesProvider", q.Op); err != nil {
		return Result{}, err
	}
	pos, done, res, err := m.resolveTarget(ctx, c, q, true)
	if err != nil {
		return Result{}, err
	}
	if done {
		return res, nil
	}
	raw, err := c.request(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
		"context":      map[string]any{"includeDeclaration": q.IncludeDeclaration},
	})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	locs, outside, err := m.normalizeLocationList(q.Op, raw)
	if err != nil {
		return Result{}, err
	}
	bounded, omitted := finalize(locs, q.Limit, compareLocation)
	res = Result{Locations: bounded, Omitted: omitted + outside, Truncated: omitted > 0}
	if outside > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("omitted %d reference(s) outside the workspace", outside))
	}
	return res, nil
}

// normalizeLocationList decodes a Location[] reply; null is an empty success.
// Workspace-escaped locations are dropped and counted instead of failing the
// whole result: one dependency hit must not hide the in-workspace references.
func (m *Manager) normalizeLocationList(op Operation, raw json.RawMessage) ([]Location, int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, 0, nil
	}
	if raw[0] != '[' {
		return nil, 0, newError(ErrProtocol, "%s: unexpected payload", op)
	}
	var arr []wireLocation
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, 0, newError(ErrProtocol, "%s: %v", op, err)
	}
	locs := make([]Location, 0, len(arr))
	outside := 0
	for _, l := range arr {
		loc, _, ok, err := locate(m.workspace, op, l)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			outside++
			continue
		}
		locs = append(locs, loc)
	}
	return locs, outside, nil
}

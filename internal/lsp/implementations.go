package lsp

import (
	"context"
	"encoding/json"
	"fmt"
)

// implementations implements textDocument/implementation: implementations of
// an interface (or interface method), and the interfaces a concrete type
// satisfies. Out-of-workspace hits — dependency types are routine here — are
// dropped and counted, never fatal.
func (m *Manager) implementations(ctx context.Context, c *client, q Query) (Result, error) {
	return m.locationLookup(ctx, c, q, "implementationProvider", "textDocument/implementation")
}

// typeDefinition implements textDocument/typeDefinition: the declaration of
// the type of the expression at the target.
func (m *Manager) typeDefinition(ctx context.Context, c *client, q Query) (Result, error) {
	return m.locationLookup(ctx, c, q, "typeDefinitionProvider", "textDocument/typeDefinition")
}

// locationLookup is the shared shape of the location-list operations that
// tolerate workspace escapes: resolve the target, ask one method, normalize,
// bound, and attach snippets.
func (m *Manager) locationLookup(
	ctx context.Context,
	c *client,
	q Query,
	capability, method string,
) (Result, error) {
	if err := c.requireCapability(capability, q.Op); err != nil {
		return Result{}, err
	}
	pos, done, res, err := m.resolveTarget(ctx, c, q, true)
	if err != nil {
		return Result{}, err
	}
	if done {
		return res, nil
	}
	raw, err := c.request(ctx, method, map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
	})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	locs, outside, err := m.normalizeLocationish(q.Op, raw)
	if err != nil {
		return Result{}, err
	}
	bounded, omitted := finalize(locs, q.Limit, compareLocation)
	m.attachSnippets(bounded)
	res = Result{Locations: bounded, Omitted: omitted + outside, Truncated: omitted > 0}
	if outside > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("omitted %d result(s) outside the workspace", outside))
	}
	return res, nil
}

// normalizeLocationish decodes a Location, Location[], or LocationLink[]
// reply — the allowed wire shapes of the goto-family methods — dropping and
// counting out-of-workspace hits instead of failing on them.
func (m *Manager) normalizeLocationish(op Operation, raw json.RawMessage) (locs []Location, outside int, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, 0, nil
	}
	appendItem := func(item json.RawMessage) error {
		var wire wireLocation
		switch {
		case jsonHas(item, "uri"):
			if err := json.Unmarshal(item, &wire); err != nil {
				return newError(ErrProtocol, "%s: %v", op, err)
			}
		case jsonHas(item, "targetUri"):
			var link wireLocationLink
			if err := json.Unmarshal(item, &link); err != nil {
				return newError(ErrProtocol, "%s: %v", op, err)
			}
			wire = wireLocation{URI: link.TargetURI, Range: link.TargetSelectionRange}
		default:
			return newError(ErrProtocol, "%s: unknown item shape", op)
		}
		loc, _, ok, err := locate(m.workspace, op, wire)
		if err != nil {
			return err
		}
		if !ok {
			outside++
			return nil
		}
		locs = append(locs, loc)
		return nil
	}
	switch raw[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, 0, newError(ErrProtocol, "%s: %v", op, err)
		}
		for _, item := range arr {
			if err := appendItem(item); err != nil {
				return nil, 0, err
			}
		}
	case '{':
		if err := appendItem(raw); err != nil {
			return nil, 0, err
		}
	default:
		return nil, 0, newError(ErrProtocol, "%s: unexpected payload", op)
	}
	return locs, outside, nil
}

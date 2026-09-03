package lsp

import (
	"context"
	"encoding/json"
	"strings"
)

// definition implements the exact-position and exact-symbol definition
// tracer. Symbol targets resolve through the document symbols first; see
// resolveTarget for the ambiguity contract.
func (m *Manager) definition(ctx context.Context, c *client, q Query) (Result, error) {
	if err := c.requireCapability("definitionProvider", q.Op); err != nil {
		return Result{}, err
	}
	pos, done, res, err := m.resolveTarget(ctx, c, q, true)
	if err != nil {
		return Result{}, err
	}
	if done {
		return res, nil
	}
	raw, err := c.request(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
	})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	locs, err := m.normalizeDefinition(raw)
	if err != nil {
		return Result{}, err
	}
	bounded, omitted := finalize(locs, q.Limit, compareLocation)
	m.attachSnippets(bounded)
	result := Result{Locations: bounded, Omitted: omitted}
	if omitted > 0 || resultTruncated(raw) {
		result.Truncated = true
	}
	return result, nil
}

// normalizeDefinition turns a Location, Location[], Location link[], or null
// into workspace-relative 1-based results. Definition stays fail-closed on
// escapes: a URI that leaves the workspace is a protocol error.
func (m *Manager) normalizeDefinition(raw json.RawMessage) ([]Location, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var locs []Location
	appendLoc := func(l wireLocation) error {
		norm, err := m.normalizeLocation(l)
		if err != nil {
			return err
		}
		locs = append(locs, norm)
		return nil
	}
	appendItem := func(item json.RawMessage) error {
		switch {
		case jsonHas(item, "uri"):
			var l wireLocation
			if err := json.Unmarshal(item, &l); err != nil {
				return newError(ErrProtocol, "definition: %v", err)
			}
			return appendLoc(l)
		case jsonHas(item, "targetUri"):
			var l wireLocationLink
			if err := json.Unmarshal(item, &l); err != nil {
				return newError(ErrProtocol, "definition: %v", err)
			}
			return appendLoc(wireLocation{URI: l.TargetURI, Range: l.TargetSelectionRange})
		default:
			return newError(ErrProtocol, "definition: unknown item shape")
		}
	}
	switch raw[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, newError(ErrProtocol, "definition: %v", err)
		}
		for _, item := range arr {
			if err := appendItem(item); err != nil {
				return nil, err
			}
		}
	case '{':
		if err := appendItem(raw); err != nil {
			return nil, err
		}
	default:
		return nil, newError(ErrProtocol, "definition: unexpected payload")
	}
	return locs, nil
}

// normalizeLocation converts one wire location, failing closed when the
// decoded path escapes the workspace.
func (m *Manager) normalizeLocation(l wireLocation) (Location, error) {
	loc, path, ok, err := locate(m.workspace, OpDefinition, l)
	if err != nil {
		return Location{}, err
	}
	if !ok {
		return Location{}, newError(ErrProtocol, "definition: %s escapes the workspace", path)
	}
	return loc, nil
}

// lineText returns the 1-based line content from a snapshot, or "".
func lineText(snapshot string, line int) string {
	if line < 1 {
		return ""
	}
	lines := strings.Split(snapshot, "\n")
	if line > len(lines) {
		return ""
	}
	return lines[line-1]
}

// readLine returns the 1-based line content of a bounded local file.
func readLine(path string, line int) (string, error) {
	raw, err := readSnapshot(path)
	if err != nil {
		return "", err
	}
	return lineText(string(raw), line), nil
}

// resultTruncated is a conservative signal that the wire payload may have
// carried more items than the limit; the Manager clamps regardless.
func resultTruncated(raw json.RawMessage) bool {
	return len(raw) > MaxTextFieldBytes
}

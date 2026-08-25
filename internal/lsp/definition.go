package lsp

import (
	"context"
	"encoding/json"
	"strings"
)

// wirePosition / wireRange / wireLocation mirror the LSP types we consume.
type wirePosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type wireRange struct {
	Start wirePosition `json:"start"`
	End   wirePosition `json:"end"`
}

type wireLocation struct {
	URI   string    `json:"uri"`
	Range wireRange `json:"range"`
}

type wireLocationLink struct {
	TargetURI            string    `json:"targetUri"`
	TargetRange          wireRange `json:"targetRange"`
	TargetSelectionRange wireRange `json:"targetSelectionRange"`
}

// definition implements the exact-position definition tracer.
func (m *Manager) definition(ctx context.Context, c *client, q Query) (Result, error) {
	snapshot, err := c.syncDocument(ctx, q.File)
	if err != nil {
		return Result{}, err
	}
	line := lineText(snapshot, q.Line)
	pos := map[string]any{
		"line":      q.Line - 1,
		"character": utf16Column(line, q.Character),
	}
	raw, err := c.request(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
	})
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return Result{}, newError(errKind(err), "definition failed: %v", err)
	}
	locs, err := m.normalizeDefinition(raw, q.Limit)
	if err != nil {
		return Result{}, err
	}
	result := Result{Locations: locs}
	if len(locs) >= q.Limit && resultTruncated(raw) {
		result.Truncated = true
	}
	return result, nil
}

// normalizeDefinition turns a Location, Location[], LocationLink[], or null
// into bounded workspace-relative 1-based results. Every returned URI is
// decoded, canonicalized, and physically contained before entering Result.
func (m *Manager) normalizeDefinition(raw json.RawMessage, limit int) ([]Location, error) {
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

	switch raw[0] {
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, newError(ErrProtocol, "definition: %v", err)
		}
		for _, item := range arr {
			if len(locs) >= limit {
				return locs[:limit], nil
			}
			switch {
			case jsonHas(item, "uri"):
				var l wireLocation
				if err := json.Unmarshal(item, &l); err != nil {
					return nil, newError(ErrProtocol, "definition: %v", err)
				}
				if err := appendLoc(l); err != nil {
					return nil, err
				}
			case jsonHas(item, "targetUri"):
				var l wireLocationLink
				if err := json.Unmarshal(item, &l); err != nil {
					return nil, newError(ErrProtocol, "definition: %v", err)
				}
				if err := appendLoc(wireLocation{URI: l.TargetURI, Range: l.TargetSelectionRange}); err != nil {
					return nil, err
				}
			default:
				return nil, newError(ErrProtocol, "definition: unknown item shape")
			}
		}
	case '{':
		switch {
		case jsonHas(raw, "uri"):
			var l wireLocation
			if err := json.Unmarshal(raw, &l); err != nil {
				return nil, newError(ErrProtocol, "definition: %v", err)
			}
			if err := appendLoc(l); err != nil {
				return nil, err
			}
		case jsonHas(raw, "targetUri"):
			var l wireLocationLink
			if err := json.Unmarshal(raw, &l); err != nil {
				return nil, newError(ErrProtocol, "definition: %v", err)
			}
			if err := appendLoc(wireLocation{URI: l.TargetURI, Range: l.TargetSelectionRange}); err != nil {
				return nil, err
			}
		default:
			return nil, newError(ErrProtocol, "definition: unknown object shape")
		}
	default:
		return nil, newError(ErrProtocol, "definition: unexpected payload")
	}
	if len(locs) > limit {
		return locs[:limit], nil
	}
	return locs, nil
}

// normalizeLocation converts one wire location into a workspace-relative
// 1-based code-point Location after physical containment.
func (m *Manager) normalizeLocation(l wireLocation) (Location, error) {
	path, err := pathFromURI(l.URI)
	if err != nil {
		return Location{}, newError(ErrProtocol, "definition: %v", err)
	}
	ok, err := contained(m.workspace, path)
	if err != nil {
		return Location{}, newError(ErrProtocol, "definition: %v", err)
	}
	if !ok {
		return Location{}, newError(ErrProtocol, "definition: %s escapes the workspace", path)
	}
	rel, err := filepathRel(m.workspace, path)
	if err != nil {
		return Location{}, newError(ErrProtocol, "definition: %v", err)
	}
	startLine := l.Range.Start.Line + 1
	endLine := l.Range.End.Line + 1
	startChar, endChar := l.Range.Start.Character, l.Range.End.Character

	line, err := readLine(path, startLine)
	if err == nil {
		startChar = codePointColumn(line, startChar)
	}
	if endLine == startLine {
		if line != "" {
			endChar = codePointColumn(line, endChar)
		}
	} else if line, err := readLine(path, endLine); err == nil {
		endChar = codePointColumn(line, endChar)
	}

	return Location{
		File:         rel,
		Line:         startLine,
		Character:    startChar,
		EndLine:      endLine,
		EndCharacter: endChar,
	}, nil
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

// jsonHas reports whether raw is an object containing key.
func jsonHas(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// resultTruncated is a conservative signal that the wire payload may have
// carried more items than the limit; the Manager clamps regardless.
func resultTruncated(raw json.RawMessage) bool {
	return len(raw) > MaxTextFieldBytes
}

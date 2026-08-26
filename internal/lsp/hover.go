package lsp

import (
	"context"
	"encoding/json"
	"strings"
)

// hover implements textDocument/hover, normalizing every allowed contents
// shape into one bounded text field with an optional range.
func (m *Manager) hover(ctx context.Context, c *client, q Query) (Result, error) {
	if err := c.requireCapability("hoverProvider", q.Op); err != nil {
		return Result{}, err
	}
	pos, done, res, err := m.resolveTarget(ctx, c, q, false)
	if err != nil {
		return Result{}, err
	}
	if done {
		return res, nil
	}
	raw, err := c.request(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
	})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	h, truncated, err := m.normalizeHover(q, raw)
	if err != nil {
		return Result{}, err
	}
	return Result{Hover: h, Truncated: truncated}, nil
}

// normalizeHover flattens the Hover envelope (or, defensively, a bare
// MarkedString payload) into a bounded Hover. null yields nil.
func (m *Manager) normalizeHover(q Query, raw json.RawMessage) (*Hover, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	contents := raw
	var rng *wireRange
	switch raw[0] {
	case '{':
		var envelope struct {
			Contents json.RawMessage `json:"contents"`
			Range    *wireRange      `json:"range"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, false, newError(ErrProtocol, "hover: %v", err)
		}
		if len(envelope.Contents) == 0 {
			return nil, false, newError(ErrProtocol, "hover: missing contents")
		}
		contents = envelope.Contents
		rng = envelope.Range
	case '"', '[':
		// Defensive: tolerate a bare MarkedString or array.
	default:
		return nil, false, newError(ErrProtocol, "hover: unexpected payload")
	}
	text, err := markedStringText(contents)
	if err != nil {
		return nil, false, err
	}
	text, truncated := boundText(text)
	h := &Hover{Text: text}
	if rng != nil {
		// The range is optional metadata in the queried file; a failed
		// conversion must not fail the hover itself.
		if loc, _, ok, err := locate(
			m.workspace,
			q.Op,
			wireLocation{URI: uriFromPath(q.File), Range: *rng},
		); err == nil &&
			ok {
			h.Line, h.Character = loc.Line, loc.Character
			h.EndLine, h.EndCharacter = loc.EndLine, loc.EndCharacter
		}
	}
	return h, truncated, nil
}

// markedStringText flattens one contents payload: a plaintext string, a
// MarkedString object (fenced when it carries a language), MarkupContent, or
// an array of these joined with newlines.
func markedStringText(raw json.RawMessage) (string, error) {
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", newError(ErrProtocol, "hover: %v", err)
		}
		return s, nil
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return "", newError(ErrProtocol, "hover: %v", err)
		}
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			s, err := markedStringText(item)
			if err != nil {
				return "", err
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n"), nil
	case '{':
		if jsonHas(raw, "kind") {
			var markup struct {
				Value string `json:"value"`
			}
			if err := json.Unmarshal(raw, &markup); err != nil {
				return "", newError(ErrProtocol, "hover: %v", err)
			}
			return markup.Value, nil
		}
		if jsonHas(raw, "value") {
			var marked struct {
				Language string `json:"language"`
				Value    string `json:"value"`
			}
			if err := json.Unmarshal(raw, &marked); err != nil {
				return "", newError(ErrProtocol, "hover: %v", err)
			}
			if marked.Language != "" {
				return "```" + marked.Language + "\n" + marked.Value + "\n```", nil
			}
			return marked.Value, nil
		}
		return "", newError(ErrProtocol, "hover: unknown contents shape")
	default:
		return "", newError(ErrProtocol, "hover: unexpected contents")
	}
}

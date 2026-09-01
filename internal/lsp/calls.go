package lsp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
)

// calls implements the two-step call hierarchy: prepare at the target
// position, then one incoming or outgoing request that replays the server's
// opaque item data verbatim.
func (m *Manager) calls(ctx context.Context, c *client, q Query) (Result, error) {
	if err := c.requireCapability("callHierarchyProvider", q.Op); err != nil {
		return Result{}, err
	}
	pos, done, res, err := m.resolveTarget(ctx, c, q, false)
	if err != nil {
		return Result{}, err
	}
	if done {
		return res, nil
	}
	raw, err := c.request(ctx, "textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		"position":     pos,
	})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	items, err := decodeCallItems(raw)
	if err != nil {
		return Result{}, err
	}
	item, err := selectCallItem(q.Op, items, pos)
	if err != nil {
		return Result{}, err
	}
	if item == nil {
		return Result{}, nil
	}
	method := "callHierarchy/incomingCalls"
	if q.Direction == DirectionOutgoing {
		method = "callHierarchy/outgoingCalls"
	}
	raw, err = c.request(ctx, method, map[string]any{"item": item})
	if err != nil {
		return Result{}, requestError(ctx, q.Op, err)
	}
	return m.normalizeCalls(q, item, method, raw)
}

func decodeCallItems(raw json.RawMessage) ([]wireCallItem, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] != '[' {
		return nil, newError(ErrProtocol, "calls: unexpected prepare payload")
	}
	var items []wireCallItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, newError(ErrProtocol, "calls: %v", err)
	}
	return items, nil
}

// selectCallItem picks the smallest item whose range contains the position.
// Equal best candidates return the standard ambiguity error. A set that does
// not contain the position at all is used as-is (defensive against servers
// with sloppy ranges) and fails ambiguous when it is not a singleton.
func selectCallItem(op Operation, items []wireCallItem, pos wirePosition) (*wireCallItem, error) {
	if len(items) == 0 {
		return nil, nil
	}
	candidates := make([]wireCallItem, 0, len(items))
	for _, item := range items {
		if rangeContains(item.Range, pos) {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 {
		candidates = items
	}
	if len(candidates) == 1 {
		return &candidates[0], nil
	}
	best := candidates[0]
	for _, cand := range candidates[1:] {
		switch compareArea(cand.Range, best.Range) {
		case -1:
			best = cand
		case 0:
			return nil, newError(ErrAmbiguous, "%s: %d equal call items contain the position", op, len(candidates))
		}
	}
	return &best, nil
}

func rangeContains(r wireRange, p wirePosition) bool {
	if p.Line < r.Start.Line || p.Line > r.End.Line {
		return false
	}
	if p.Line == r.Start.Line && p.Character < r.Start.Character {
		return false
	}
	if p.Line == r.End.Line && p.Character > r.End.Character {
		return false
	}
	return true
}

// compareArea orders ranges by span (lines, then characters) so the smallest
// containing item wins deterministically.
func compareArea(a, b wireRange) int {
	return cmp.Or(
		cmp.Compare(a.End.Line-a.Start.Line, b.End.Line-b.Start.Line),
		cmp.Compare(a.End.Character-a.Start.Character, b.End.Character-b.Start.Character),
	)
}

// normalizeCalls turns incoming/outgoing replies into bounded edges. Call
// sites live in the caller's document: the prepared item for outgoing, the
// remote item for incoming.
func (m *Manager) normalizeCalls(q Query, prepared *wireCallItem, method string, raw json.RawMessage) (Result, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return Result{}, nil
	}
	if raw[0] != '[' {
		return Result{}, newError(ErrProtocol, "calls: unexpected payload")
	}
	var calls []Call
	outside := 0
	appendEdge := func(from, to wireCallItem, site wireLocation) error {
		fromSym, okFrom, err := m.callSymbol(q.Op, from)
		if err != nil {
			return err
		}
		toSym, okTo, err := m.callSymbol(q.Op, to)
		if err != nil {
			return err
		}
		siteLoc, _, okSite, err := locate(m.workspace, q.Op, site)
		if err != nil {
			return err
		}
		if !okFrom || !okTo || !okSite {
			outside++
			return nil
		}
		calls = append(calls, Call{From: fromSym, To: toSym, Location: siteLoc})
		return nil
	}
	if method == "callHierarchy/incomingCalls" {
		var arr []wireIncomingCall
		if err := json.Unmarshal(raw, &arr); err != nil {
			return Result{}, newError(ErrProtocol, "calls: %v", err)
		}
		for _, call := range arr {
			for _, r := range call.FromRanges {
				if err := appendEdge(call.From, *prepared, wireLocation{URI: call.From.URI, Range: r}); err != nil {
					return Result{}, err
				}
			}
		}
	} else {
		var arr []wireOutgoingCall
		if err := json.Unmarshal(raw, &arr); err != nil {
			return Result{}, newError(ErrProtocol, "calls: %v", err)
		}
		for _, call := range arr {
			for _, r := range call.FromRanges {
				if err := appendEdge(*prepared, call.To, wireLocation{URI: prepared.URI, Range: r}); err != nil {
					return Result{}, err
				}
			}
		}
	}
	bounded, omitted := finalize(calls, q.Limit, compareCall)
	m.attachCallSnippets(bounded)
	res := Result{Calls: bounded, Omitted: omitted + outside, Truncated: omitted > 0}
	if outside > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("omitted %d call(s) outside the workspace", outside))
	}
	return res, nil
}

// callSymbol normalizes one hierarchy item; ok=false means its file fell
// outside the workspace.
func (m *Manager) callSymbol(op Operation, item wireCallItem) (Symbol, bool, error) {
	loc, _, ok, err := locate(m.workspace, op, wireLocation{URI: item.URI, Range: item.SelectionRange})
	if err != nil || !ok {
		return Symbol{}, ok, err
	}
	name, _ := boundText(item.Name)
	detail, _ := boundText(item.Detail)
	return Symbol{Name: name, Kind: symbolKindName(item.Kind), Detail: detail, Location: loc}, true, nil
}

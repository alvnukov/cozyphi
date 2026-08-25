package lsp

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"unicode/utf8"
)

// finalize deduplicates by normalized identity, sorts deterministically, and
// applies the frozen item limit. It reports how many unique results the limit
// dropped so callers can populate Result.Omitted.
func finalize[T comparable](items []T, limit int, compare func(a, b T) int) ([]T, int) {
	slices.SortStableFunc(items, compare)
	out := make([]T, 0, len(items))
	seen := make(map[T]struct{}, len(items))
	for _, item := range items {
		if _, dup := seen[item]; dup {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if len(out) > limit {
		omitted := len(out) - limit
		return out[:limit], omitted
	}
	return out, 0
}

// compareLocation orders by path, start line, start character, then end.
func compareLocation(a, b Location) int {
	return cmp.Or(
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Character, b.Character),
		cmp.Compare(a.EndLine, b.EndLine),
		cmp.Compare(a.EndCharacter, b.EndCharacter),
	)
}

func compareSymbol(a, b Symbol) int {
	return cmp.Or(
		compareLocation(a.Location, b.Location),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.Kind, b.Kind),
	)
}

func compareCall(a, b Call) int {
	return cmp.Or(
		compareLocation(a.Location, b.Location),
		cmp.Compare(a.From.Name, b.From.Name),
		cmp.Compare(a.To.Name, b.To.Name),
	)
}

// boundText truncates s to MaxTextFieldBytes on a rune boundary and reports
// whether it cut anything. Normalized text fields are bounded before entering
// Result.
func boundText(s string) (string, bool) {
	if len(s) <= MaxTextFieldBytes {
		return s, false
	}
	cut := s[:MaxTextFieldBytes]
	for cut != "" && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

// requestError converts a failed wire request into a typed error unless the
// caller's context is the cause, in which case the raw ctx error wins so
// cancellation stays discoverable via errors.Is.
func requestError(ctx context.Context, op Operation, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return newError(errKind(err), "%s failed: %v", op, err)
}

// locate decodes one wire location, contains it physically, and converts it
// to a workspace-relative 1-based code-point Location. ok=false with a nil
// error means the decoded path (returned in path) fell outside the workspace:
// definition treats that as a protocol error, the navigation ops drop the
// result and count it instead.
func (m *Manager) locate(op Operation, l wireLocation) (loc Location, path string, ok bool, err error) {
	path, err = pathFromURI(l.URI)
	if err != nil {
		return Location{}, "", false, newError(ErrProtocol, "%s: %v", op, err)
	}
	inside, err := contained(m.workspace, path)
	if err != nil {
		return Location{}, path, false, newError(ErrProtocol, "%s: %v", op, err)
	}
	if !inside {
		return Location{}, path, false, nil
	}
	rel, err := filepathRel(m.workspace, path)
	if err != nil {
		return Location{}, path, false, newError(ErrProtocol, "%s: %v", op, err)
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
	} else if endLineText, err := readLine(path, endLine); err == nil {
		endChar = codePointColumn(endLineText, endChar)
	}

	return Location{
		File:         rel,
		Line:         startLine,
		Character:    startChar,
		EndLine:      endLine,
		EndCharacter: endChar,
	}, path, true, nil
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

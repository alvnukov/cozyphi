package diag

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
)

// bounder sanitizes and bounds everything on its way out of the module. It
// lives in the registry rather than in collectors on purpose: sixteen more
// collectors are coming, and a rule enforced once at the boundary cannot be
// forgotten by any of them. A collector that hands over a raw API key, an
// absolute home path or a terminal escape sequence still produces a safe
// answer.
type bounder struct {
	limits Limits
	home   string
}

func newBounder(limits Limits, home string) *bounder {
	return &bounder{limits: limits, home: home}
}

// text runs one string through the full pipeline: mask known secret shapes,
// collapse the user's home directory to ~, drop control characters, then cut
// to the byte cap. It reports whether the string had to be masked and
// whether it had to be cut.
func (b *bounder) text(s string) (out string, masked, truncated bool) {
	if s == "" {
		return "", false, false
	}
	out = redact.Redact(s)
	masked = out != s
	if b.home != "" {
		out = strings.ReplaceAll(out, b.home, "~")
	}
	out = stripControl(out)
	if len(out) > b.limits.MaxValueBytes {
		out = cutBytes(out, b.limits.MaxValueBytes)
		truncated = true
	}
	return out, masked, truncated
}

// value sanitizes and bounds one value, including every list item. The list
// is always rebuilt, so the returned value never aliases the collector's.
func (b *bounder) value(v Value) (out Value, masked, truncated bool) {
	out = v
	str, strMasked, strTruncated := b.text(v.Str)
	out.Str = str
	masked = strMasked
	truncated = strTruncated

	keep := min(len(v.List), b.limits.MaxListItems)
	if keep < len(v.List) {
		truncated = true
	}
	list := make([]string, 0, keep)
	for _, item := range v.List[:keep] {
		text, itemMasked, itemTruncated := b.text(item)
		masked = masked || itemMasked
		truncated = truncated || itemTruncated
		list = append(list, text)
	}
	out.List = list
	return out, masked, truncated
}

// observation sanitizes one layer. A value that had to be masked is reported
// as StateRedacted, so the model is told a real value exists and was withheld
// rather than being shown something that looks like the value.
func (b *bounder) observation(o Observation) (out Observation, truncated bool) {
	out = o
	value, masked, valueTruncated := b.value(o.Value)
	out.Value = value
	ref, _, refTruncated := b.text(o.Source.Ref)
	out.Source.Ref = ref
	if masked && out.State == StatePresent {
		out.State = StateRedacted
	}
	return out, valueTruncated || refTruncated
}

// field sanitizes a whole field and stamps it with the category's observation
// time. Collectors never set ObservedAt: the registry owns the clock, so one
// category's fields always agree with each other.
//
// Freshness is normalized here for the same reason: a collector that says
// nothing about it has, by the Collector contract, read its owner during
// this observation, and saying so once at the boundary is what keeps the
// field from being empty on every collector that never thought about it.
func (b *bounder) field(f Field) (out Field, truncated bool) {
	out = f
	if out.Freshness == "" {
		out.Freshness = FreshnessLive
	}
	key, _, keyTruncated := b.text(f.Key)
	out.Key = key
	configured, configuredTruncated := b.observation(f.Configured)
	loaded, loadedTruncated := b.observation(f.Loaded)
	effective, effectiveTruncated := b.observation(f.Effective)
	out.Configured = configured
	out.Loaded = loaded
	out.Effective = effective
	revision, _, revisionTruncated := b.text(f.Revision)
	out.Revision = revision
	out, cut := b.fit(out)
	return out, cut || keyTruncated || configuredTruncated || loadedTruncated ||
		effectiveTruncated || revisionTruncated
}

// rowCost is what one row costs the answer: the bytes it will be rendered as,
// plus the comma that joins it to the next one. It takes the row itself —
// a Field in a detail view, an OverviewField in an overview — so the thing
// charged for and the thing emitted are the same value, and an overview row
// is cheap because it really is smaller rather than because a constant said
// so.
func rowCost(row any) int { return marshaledLen(row) + 1 }

// envelope is what one category costs before a single row is in it: its name,
// availability, reason and timestamp, and the comma after it.
func envelope(entry CategorySnapshot) int {
	entry.Overview, entry.Fields = nil, nil
	return marshaledLen(entry) + 1
}

// answerEnvelope is what a snapshot costs before a single category is in it:
// the category asked for, the mode, the two flags and the note. The flags and
// the note are charged at their widest, because both are written after the
// categories have been read and there is nothing left to take them out of by
// then, and an answer that overran its cap only when it was cut would be the
// one case the cap exists for.
func answerEnvelope(s Snapshot) int {
	s.Categories = nil
	s.Truncated, s.Partial = true, true
	s.Note = note(true, true)
	return marshaledLen(s)
}

// Bounds on a single field, which is what an explain answer is made of.
const (
	// envelopeReserveBytes is held back from MaxTotalBytes when one field is
	// bounded, to cover what any answer wraps a field in: the category, its
	// availability and reason, a timestamp and the flags. It is a round
	// reserve rather than a measurement because the exact envelope is
	// charged separately; all this number has to guarantee is that a field
	// small enough to pass really does leave room for one.
	envelopeReserveBytes = 1024
	// minFieldBytes is the smallest ceiling a field is ever held to. Three
	// layers of a value and a source is a few hundred bytes before any
	// content, so below this there is nothing dropping list items can
	// achieve, and a caller who set an unusably small MaxTotalBytes gets one
	// whole field rather than a shredded one — an answer smaller than a
	// single field is not an answer.
	minFieldBytes = 4096
)

// fit shrinks one field until it can be rendered inside a single answer.
// Explain returns exactly one field, so a field that cannot fit the cap is
// an answer that cannot honor it, whatever the snapshot budget does.
//
// Only a list can make a field large enough to need this: every string on it
// is already cut to MaxValueBytes, so the one multiplier left is MaxListItems
// items of that size across three layers, which is more than a whole answer
// may cost. Items are dropped from the longest list until the field fits, and
// the caller is told, because a field nobody can be given is worse than a
// field with fewer items in it.
func (b *bounder) fit(f Field) (Field, bool) {
	ceiling := max(b.limits.MaxTotalBytes-envelopeReserveBytes, minFieldBytes)
	if marshaledLen(f) <= ceiling {
		return f, false
	}
	for marshaledLen(f) > ceiling {
		longest := longestList(&f)
		if longest == nil {
			break
		}
		longest.Value.List = longest.Value.List[:len(longest.Value.List)-1]
	}
	return f, true
}

// longestList points at whichever layer carries the most list items, or nil
// when no layer has any left to drop.
func longestList(f *Field) *Observation {
	var longest *Observation
	for _, layer := range []*Observation{&f.Configured, &f.Loaded, &f.Effective} {
		if len(layer.Value.List) == 0 {
			continue
		}
		if longest == nil || len(layer.Value.List) > len(longest.Value.List) {
			longest = layer
		}
	}
	return longest
}

// stripControl removes control characters. Terminal escapes, newlines and
// stray NULs have no business in a value that is about to be rendered into a
// transcript the user reads.
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// cutBytes trims s to at most limit bytes, on a rune boundary, ending with a
// marker so a cut value never reads as a complete one.
func cutBytes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	if limit <= len(truncationMarker) {
		return truncationMarker[:limit]
	}
	head := s[:limit-len(truncationMarker)]
	for head != "" && !utf8.ValidString(head) {
		head = head[:len(head)-1]
	}
	return head + truncationMarker
}

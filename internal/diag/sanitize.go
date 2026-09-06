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
	spent  int
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
func (b *bounder) field(f Field) (out Field, truncated bool) {
	out = f
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
	return out, keyTruncated || configuredTruncated || loadedTruncated || effectiveTruncated || revisionTruncated
}

// afford charges cost against the total byte budget and reports whether it
// fit. A response that would cross the cap stops adding fields and says so,
// which is why there is no pagination protocol: the caller narrows by
// category and key instead.
func (b *bounder) afford(cost int) bool {
	if b.spent+cost > b.limits.MaxTotalBytes {
		return false
	}
	b.spent += cost
	return true
}

// observationCost approximates one layer's rendered size.
func observationCost(o Observation) int {
	cost := len(o.State) + len(o.Value.Kind) + len(o.Value.Str) + len(o.Source.Kind) + len(o.Source.Ref)
	for _, item := range o.Value.List {
		cost += len(item) + 3
	}
	return cost
}

// fieldCost approximates one detail field's rendered size.
func fieldCost(f Field) int {
	return len(f.Key) + len(f.Apply) + len(f.Scope) + len(f.Revision) + fieldOverheadBytes +
		observationCost(f.Configured) + observationCost(f.Loaded) + observationCost(f.Effective)
}

// overviewCost approximates one overview row's rendered size.
func overviewCost(f Field) int {
	return len(f.Key) + observationCost(f.Effective) + fieldOverheadBytes/4
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

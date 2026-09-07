package diag

import (
	"encoding/json"
	"math"
)

// JSON rendering is deterministic by construction: every output DTO is a
// struct, so field order is the declaration order the reader can rely on;
// there are no maps anywhere in the output, so nothing depends on Go's map
// iteration; and no field carries omitempty, so a false, a 0 or an empty
// list reaches the model as itself rather than as a hole it has to guess at.

// JSON renders the catalog.
func (c Catalog) JSON() string { return render(c) }

// JSON renders the snapshot.
func (s Snapshot) JSON() string { return render(s) }

// JSON renders the explanation.
func (e Explanation) JSON() string { return render(e) }

// render marshals an output DTO. The DTOs contain only structs, strings,
// numbers, bools, string slices and time.Time, none of which can fail to
// marshal; the fallback exists so a future field can never turn a
// diagnostics answer into a panic or an empty body.
//
// The output is compact. It used to be indented, and on a structure this
// deeply nested — a field is four levels down, and carries three layers with
// a value and a source apiece — the leading spaces were some forty-five per
// cent of every answer. They were bought for a reader who does not exist:
// what is rendered here goes to a model as tool output, and the person who
// wants to read it has a formatter. Spending nearly half of a bounded budget
// on whitespace meant spending it on fields that had to be dropped instead.
func render(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"error":"diagnostics could not be rendered"}`
	}
	return string(data)
}

// marshaledLen is how many bytes a value will occupy in an answer. The
// budget is charged with this rather than with an estimate built out of
// string lengths and a constant: an estimate of a structure this shape drifts
// from the structure the moment a member is added to it, and the one it
// drifted from was three times under, which is how a cap that was supposed to
// bind stopped binding. Measuring costs one marshal of a small struct and
// cannot drift at all.
//
// A value that cannot be marshaled is charged more than any budget can hold,
// so it is refused rather than admitted for free — the same DTOs render
// through the fallback above, so nothing can be both unmeasurable and
// emitted.
func marshaledLen(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return math.MaxInt32
	}
	return len(data)
}

package diag

import "encoding/json"

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
func render(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return `{"error":"diagnostics could not be rendered"}`
	}
	return string(data)
}

package tooldef

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// gateArgKeys are harness-owned argument keys the executor consumes before
// dispatch; the schema the model saw listed them, so validating a call's own
// arguments must accept them too. The plan gate also judges their absence
// itself, so a schema that requires one is not enforced here.
var gateArgKeys = map[string]struct{}{"plan_step": {}}

// argAliases maps an argument key some models send to the declared key it
// stands for. A schema that declares the canonical key accepts the alias in
// its place: edit resolves file_path itself, and a tool that does not still
// reports the missing path in its own words rather than as an unknown key.
var argAliases = map[string]string{"file_path": "path"}

// ValidateAgainstSchema checks raw arguments against the JSON Schema shape
// the model was shown: one JSON object whose top-level keys are declared
// properties (or harness-owned gate keys) with required keys present and
// declared scalar types honored. The executor runs it on every call before
// any gate, so a malformed call is refused with the declared keys named
// instead of reaching a user prompt or the tool; the tool's own strict decode
// stays the authority once the call runs. A tool without a schema declared
// nothing and is left to its decoder.
func ValidateAgainstSchema(raw json.RawMessage, params *llm.FunctionParameters) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return fmt.Errorf("arguments must be one JSON object: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values in arguments")
		}
		return err
	}
	if params == nil {
		return nil
	}
	props := params.Properties
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	present := make(map[string]struct{}, len(fields))
	for _, key := range keys {
		if _, gate := gateArgKeys[key]; gate {
			continue
		}
		declared := key
		prop, ok := lookupProp(props, key)
		if !ok {
			if canonical, aliased := argAliases[key]; aliased {
				prop, ok = lookupProp(props, canonical)
				declared = canonical
			}
		}
		if !ok {
			return fmt.Errorf("unknown argument %q; declared: %s", key, declaredKeys(props))
		}
		present[declared] = struct{}{}
		if err := checkSchemaKind(key, fields[key], prop); err != nil {
			return err
		}
	}
	for _, required := range params.Required {
		if _, gate := gateArgKeys[required]; gate {
			continue
		}
		if _, ok := present[required]; !ok {
			return fmt.Errorf("missing required argument %q", required)
		}
	}
	return nil
}

func lookupProp(props llm.Object, key string) (any, bool) {
	if props == nil {
		return nil, false
	}
	prop, ok := props[key]
	return prop, ok
}

// declaredKeys names the schema's properties for a refusal, so the model can
// correct the call from the result alone.
func declaredKeys(props llm.Object) string {
	if len(props) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(props))
	for key := range props {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

func checkSchemaKind(key string, value json.RawMessage, prop any) error {
	typeNames := declaredTypes(prop)
	if len(typeNames) == 0 {
		return nil // no type constraint in the schema; the tool's decode judges
	}
	kind := jsonKind(value)
	for _, name := range typeNames {
		if kindAllowed(kind, name, value) {
			return nil
		}
	}
	return fmt.Errorf("argument %q must be %s, not %s", key, joinTypeNames(typeNames), kind)
}

func declaredTypes(prop any) []string {
	obj, ok := prop.(map[string]any)
	if !ok {
		return nil
	}
	switch t := obj["type"].(type) {
	case string:
		return []string{t}
	case []any:
		var names []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
		return names
	default:
		return nil
	}
}

func joinTypeNames(names []string) string {
	return strings.Join(names, " or ")
}

// jsonKind names the JSON value kind for schema comparison.
func jsonKind(value json.RawMessage) string {
	for _, b := range value {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '"':
			return "string"
		case '{':
			return "object"
		case '[':
			return "array"
		case 't', 'f':
			return "boolean"
		case 'n':
			return "null"
		default:
			return "number"
		}
	}
	return "null"
}

func kindAllowed(kind, want string, value json.RawMessage) bool {
	switch {
	case kind == want:
		return true
	case kind == "null":
		return true // an explicit null may clear an optional slot; the tool's decode judges
	case want == "integer" && kind == "number":
		num, err := strconv.ParseFloat(string(bytes.TrimSpace(value)), 64)
		return err == nil && num == float64(int64(num))
	default:
		return false
	}
}

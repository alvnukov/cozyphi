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
// arguments must accept them too.
var gateArgKeys = map[string]struct{}{"plan_step": {}}

// ValidateAgainstSchema checks raw arguments against the JSON Schema shape
// the model was shown: one JSON object whose top-level keys are declared
// properties (or harness-owned gate keys) with required keys present and
// declared scalar types honored. It is the pre-dispatch half of argument
// validation — the tool's own strict decode stays the authority once the
// call runs — so a piggybacked plan settle can refuse to land on a call the
// tool would have rejected anyway.
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
	props := llm.Object(nil)
	if params != nil {
		props = params.Properties
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, gate := gateArgKeys[key]; gate {
			continue
		}
		prop, declared := lookupProp(props, key)
		if !declared {
			return fmt.Errorf("unknown argument %q", key)
		}
		if err := checkSchemaKind(key, fields[key], prop); err != nil {
			return err
		}
	}
	if params != nil {
		for _, required := range params.Required {
			if _, present := fields[required]; !present {
				return fmt.Errorf("missing required argument %q", required)
			}
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

package tooldef

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func schemaParams() *llm.FunctionParameters {
	return &llm.FunctionParameters{
		Type: "object",
		Properties: llm.Object{
			"path":   map[string]any{"type": "string"},
			"count":  map[string]any{"type": "integer"},
			"strict": map[string]any{"type": "boolean"},
			"tags":   map[string]any{"type": "array"},
		},
		Required: []string{"path"},
	}
}

func TestValidateAgainstSchemaAccepts(t *testing.T) {
	params := schemaParams()
	for name, raw := range map[string]string{
		"full":          `{"path":"a.go","count":2,"strict":true,"tags":["x"]}`,
		"empty":         ``,
		"gate key only": `{"path":"a.go","plan_step":"wire"}`,
		"null optional": `{"path":"a.go","count":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, ValidateAgainstSchema(json.RawMessage(raw), params))
		})
	}
}

func TestValidateAgainstSchemaRejects(t *testing.T) {
	params := schemaParams()
	cases := map[string]string{
		"unknown key":       `{"path":"a.go","bogus":"y"}`,
		"missing required":  `{"count":2}`,
		"wrong scalar kind": `{"path":3}`,
		"integer as float":  `{"count":1.5}`,
		"integer as text":   `{"count":"2"}`,
		"multiple values":   `{"path":"a.go"}{"path":"b.go"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, ValidateAgainstSchema(json.RawMessage(raw), params))
		})
	}
}

func TestValidateAgainstSchemaNoSchemaDeclaresNothing(t *testing.T) {
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{}`), nil))
	assert.Error(t, ValidateAgainstSchema(json.RawMessage(`{"path":"a.go"}`), nil))
}

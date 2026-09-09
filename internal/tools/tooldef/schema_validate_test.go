package tooldef

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestValidateAgainstSchemaWithoutSchemaLeavesArgsToTool(t *testing.T) {
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{}`), nil))
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{"path":"a.go"}`), nil),
		"no schema declared nothing; the tool's decoder judges")
	assert.Error(t, ValidateAgainstSchema(json.RawMessage(`[1]`), nil),
		"the one-object shape holds for every tool")
	declaresNothing := &llm.FunctionParameters{Type: "object"}
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{}`), declaresNothing))
	assert.Error(t, ValidateAgainstSchema(json.RawMessage(`{"path":"a.go"}`), declaresNothing),
		"an object schema with no properties declares that no argument exists")
}

func TestValidateAgainstSchemaNamesDeclaredKeysOnUnknown(t *testing.T) {
	err := ValidateAgainstSchema(json.RawMessage(`{"path":"a.go","bogus":"y"}`), schemaParams())
	require.EqualError(t, err, `unknown argument "bogus"; declared: count, path, strict, tags`)
	err = ValidateAgainstSchema(json.RawMessage(`{"bogus":"y"}`), &llm.FunctionParameters{Type: "object"})
	require.EqualError(t, err, `unknown argument "bogus"; declared: none`)
}

func TestValidateAgainstSchemaLeavesGateKeysToTheGate(t *testing.T) {
	params := schemaParams()
	params.Required = append(params.Required, "plan_step")
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{"path":"a.go"}`), params),
		"a missing plan_step is the plan gate's verdict, not a schema refusal")
	assert.Error(t, ValidateAgainstSchema(json.RawMessage(`{"plan_step":"wire"}`), params),
		"the gate key does not stand in for the tool's own required keys")
}

func TestValidateAgainstSchemaAcceptsFilePathAlias(t *testing.T) {
	params := schemaParams()
	assert.NoError(t, ValidateAgainstSchema(json.RawMessage(`{"file_path":"a.go"}`), params),
		"file_path stands for the declared path and satisfies its requirement")
	assert.EqualError(t, ValidateAgainstSchema(json.RawMessage(`{"file_path":3}`), params),
		`argument "file_path" must be string, not number`,
		"the alias is held to the declared key's type")
	noPath := &llm.FunctionParameters{Type: "object", Properties: llm.Object{"url": map[string]any{"type": "string"}}}
	assert.Error(t, ValidateAgainstSchema(json.RawMessage(`{"file_path":"a.go"}`), noPath),
		"the alias only stands for a key the schema declares")
}

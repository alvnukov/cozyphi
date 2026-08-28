package tooldef

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeStrict_DecodesSingleValue(t *testing.T) {
	var in struct {
		Action string `json:"action"`
	}
	require.NoError(t, DecodeStrict([]byte(`{"action":"start"}`), &in))
	assert.Equal(t, "start", in.Action)
}

func TestDecodeStrict_EmptyInputKeepsDefaults(t *testing.T) {
	for _, raw := range []string{``, `   `, `{}`} {
		in := struct {
			Action string `json:"action"`
		}{Action: "list"}
		require.NoError(t, DecodeStrict([]byte(raw), &in), "raw %q", raw)
		assert.Equal(t, "list", in.Action)
	}
}

func TestDecodeStrict_RejectsUnknownFields(t *testing.T) {
	var in struct {
		Action string `json:"action"`
	}
	err := DecodeStrict([]byte(`{"action":"start","command":"ls"}`), &in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestDecodeStrict_RejectsTrailingData(t *testing.T) {
	for name, raw := range map[string]string{
		"second value": `{"action":"start"} {"action":"stop"}`,
		"garbage":      `{"action":"start"} oops`,
	} {
		var in struct {
			Action string `json:"action"`
		}
		err := DecodeStrict([]byte(raw), &in)
		require.Error(t, err, name)
	}
}

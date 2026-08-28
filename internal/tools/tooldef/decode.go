package tooldef

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// DecodeStrict decodes raw tool arguments into dst under the single tool
// convention: unknown fields are rejected, exactly one JSON value is allowed,
// and empty input decodes as {} so defaults pre-set in dst survive. Callers
// wrap the error with their tool-name prefix.
func DecodeStrict(raw json.RawMessage, dst any) error {
	if strings.TrimSpace(string(raw)) == "" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

// PathArgs is the path-bearing subset of tool arguments. FilePath is the
// alias some models send for edit in place of path.
type PathArgs struct {
	Path     string `json:"path"`
	FilePath string `json:"file_path"`
}

// Resolved prefers path and falls back to the file_path alias.
func (a PathArgs) Resolved() string {
	if a.Path != "" {
		return a.Path
	}
	return a.FilePath
}

// FilePathAlias returns the file_path alias from raw tool arguments, empty
// when absent or undecodable. Edit falls back to it when path is missing.
func FilePathAlias(raw json.RawMessage) string {
	var in PathArgs
	if json.Unmarshal(raw, &in) != nil {
		return ""
	}
	return in.FilePath
}

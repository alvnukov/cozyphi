package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CallResult contains the MCP fields visible to the caller. Server-private
// metadata is excluded even when no textual summary accompanied the result.
type CallResult struct {
	Content           []ContentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
	IsError           bool            `json:"isError"`
}

// ContentBlock describes text, embedded media, a resource or a resource link.
// Pool.CallResult replaces embedded bytes with a bounded, local artifact handle.
type ContentBlock struct {
	Type        string    `json:"type"`
	Text        string    `json:"text,omitempty"`
	MIMEType    string    `json:"mimeType,omitempty"`
	Data        string    `json:"data,omitempty"`
	URI         string    `json:"uri,omitempty"`
	Name        string    `json:"name,omitempty"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Size        *int64    `json:"size,omitempty"`
	Resource    *Resource `json:"resource,omitempty"`
	Artifact    *Artifact `json:"artifact,omitempty"`
}

type Resource struct {
	URI      string    `json:"uri"`
	MIMEType string    `json:"mimeType,omitempty"`
	Text     string    `json:"text,omitempty"`
	Blob     string    `json:"blob,omitempty"`
	Artifact *Artifact `json:"artifact,omitempty"`
}

func decodeCallResult(raw json.RawMessage) (CallResult, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return CallResult{}, errors.New("decode MCP tool result: expected a JSON object")
	}
	var result CallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CallResult{}, fmt.Errorf("decode MCP tool result: %w", err)
	}
	if result.Content == nil {
		result.Content = []ContentBlock{}
	}
	// A remote server cannot mint local handles, even by echoing an old one.
	for i := range result.Content {
		block := &result.Content[i]
		block.Artifact = nil
		if block.Resource != nil {
			block.Resource.Artifact = nil
		}
	}
	return result, nil
}

// Text gives direct callers the usual compact text. Binary payloads never
// enter transcripts as base64. A media-only result identifies its type.
func (r CallResult) Text() string {
	var b strings.Builder
	for _, item := range r.Content {
		switch item.Type {
		case "", "text":
			b.WriteString(item.Text)
		case "resource":
			if item.Resource != nil {
				b.WriteString(item.Resource.Text)
			}
		}
	}
	if b.Len() == 0 {
		switch {
		case len(r.StructuredContent) != 0:
			b.Write(r.StructuredContent)
		case len(r.Content) != 0:
			for _, item := range r.Content {
				fmt.Fprintf(&b, "[%s", item.Type)
				if item.URI != "" {
					fmt.Fprintf(&b, " %s", item.URI)
				} else if item.Resource != nil && item.Resource.URI != "" {
					fmt.Fprintf(&b, " %s", item.Resource.URI)
				}
				ref := item.Artifact
				if item.Resource != nil && item.Resource.Artifact != nil {
					ref = item.Resource.Artifact
				}
				if ref != nil && ref.ID != "" {
					fmt.Fprintf(&b, " artifact=%s bytes=%d", ref.ID, ref.Size)
				}
				b.WriteByte(']')
			}
		default:
			b.WriteString("(empty MCP tool result)")
		}
	}
	if r.IsError {
		return "error: " + b.String()
	}
	return b.String()
}

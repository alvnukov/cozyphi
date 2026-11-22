package mcp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	artifactStoreBytes       = 16 << 20
	artifactStoreCount       = 128
	artifactPageBytes        = 128 << 10
	defaultArtifactPageBytes = 16 << 10
	inlineResultBytes        = 1 << 20
)

// Artifact identifies embedded wire data held only by this connection pool.
type Artifact struct {
	ID       string `json:"id,omitempty"`
	Size     int    `json:"size"`
	Encoding string `json:"encoding,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Error    string `json:"error,omitempty"`
}

type storedArtifact struct {
	server string
	tool   string
	ref    Artifact
	bytes  []byte
}

func (p *Pool) retainArtifacts(server, tool string, result *CallResult) {
	for i := range result.Content {
		block := &result.Content[i]
		if block.Data != "" {
			block.Artifact = p.storeArtifact(server, tool, block.MIMEType, block.Data)
			block.Data = ""
		}
		if r := block.Resource; r != nil && r.Blob != "" {
			r.Artifact = p.storeArtifact(server, tool, r.MIMEType, r.Blob)
			r.Blob = ""
		}
	}
}

func (p *Pool) storeArtifact(server, tool, mime, encoded string) *Artifact {
	ref := Artifact{MIMEType: mime, Encoding: "base64"}
	if base64.StdEncoding.DecodedLen(len(encoded)) > artifactStoreBytes {
		ref.Error = "artifact exceeds the pool byte limit"
		return &ref
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		ref.Error = "invalid base64 payload"
		return &ref
	}
	return p.retain(server, tool, ref, data)
}

func (p *Pool) retain(server, tool string, ref Artifact, data []byte) *Artifact {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.disabled[server] {
		ref.Error = "MCP server is unavailable"
		return &ref
	}
	ref.ID = rand.Text()
	ref.Size = len(data)
	used := len(data)
	for _, item := range p.artifacts {
		used += len(item.bytes)
	}
	for len(p.artifacts) >= artifactStoreCount || used > artifactStoreBytes {
		used -= len(p.artifacts[0].bytes)
		p.artifacts[0] = storedArtifact{}
		p.artifacts = p.artifacts[1:]
	}
	p.artifacts = append(p.artifacts, storedArtifact{server: server, tool: tool, ref: ref, bytes: data})
	return &ref
}

// ProgramResult keeps large valid JSON responses usable within a cell's
// per-call budget. Paging a snapshot always reads exactly that response and
// never repeats a possibly mutating tools/call.
func (p *Pool) ProgramResult(server, tool string, result CallResult) (any, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode MCP result: %w", err)
	}
	if len(data) <= inlineResultBytes {
		return json.RawMessage(data), nil
	}
	if len(data) > artifactStoreBytes {
		return nil, fmt.Errorf(
			"encoded MCP result exceeds %d bytes; request a smaller page from the tool",
			artifactStoreBytes,
		)
	}
	ref := p.retain(server, tool, Artifact{MIMEType: "application/json", Encoding: "utf8"}, data)
	if ref.Error != "" {
		return nil, errors.New(ref.Error)
	}
	return map[string]any{"isError": result.IsError, "resultArtifact": ref}, nil
}

// ArtifactPage holds at most one page. Offsets are decoded bytes.
type ArtifactPage struct {
	Artifact
	Offset int    `json:"offset"`
	Next   *int   `json:"nextOffset,omitempty"`
	Data   string `json:"data,omitempty"`
	Text   string `json:"text,omitempty"`
}

// ReadArtifact reads a local capability issued for exactly this server/tool.
// Disabling a server closes access just as it does for a live call.
func (p *Pool) ReadArtifact(server, tool, id string, offset, limit int) (ArtifactPage, error) {
	if offset < 0 || limit < 0 || limit > artifactPageBytes {
		return ArtifactPage{}, fmt.Errorf(
			"invalid artifact range: offset must be nonnegative and limit at most %d bytes",
			artifactPageBytes,
		)
	}
	if limit == 0 {
		limit = defaultArtifactPageBytes
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.disabled[server] {
		return ArtifactPage{}, errors.New("MCP server unavailable for artifact read")
	}
	for _, item := range p.artifacts {
		if item.server != server || item.tool != tool || item.ref.ID != id {
			continue
		}
		if offset > len(item.bytes) {
			return ArtifactPage{}, errors.New("artifact offset exceeds its size")
		}
		end := offset + min(limit, len(item.bytes)-offset)
		page := ArtifactPage{Artifact: item.ref, Offset: offset}
		if item.ref.Encoding == "utf8" {
			if offset < len(item.bytes) && !utf8.RuneStart(item.bytes[offset]) {
				return ArtifactPage{}, errors.New("artifact offset splits a UTF-8 character; use nextOffset")
			}
			for end < len(item.bytes) && !utf8.RuneStart(item.bytes[end]) {
				end--
			}
			if end == offset && end < len(item.bytes) {
				return ArtifactPage{}, errors.New("artifact limit is too small for the next UTF-8 character")
			}
			page.Text = string(item.bytes[offset:end])
		} else {
			page.Data = base64.StdEncoding.EncodeToString(item.bytes[offset:end])
		}
		if end < len(item.bytes) {
			page.Next = &end
		}
		return page, nil
	}
	return ArtifactPage{}, errors.New("unknown or expired artifact for this server and tool")
}

// Package configfile owns the read-modify-write cycle of a single YAML config
// file (~/.cozyphi/config.yaml). Every writer in the process commits through
// [Edit], which serializes cycles on the same path and writes atomically with
// owner-only permissions: two editors cannot interleave their read and write,
// and a crash never leaves a torn file. Unrelated keys, comments, and ordering
// survive verbatim — editors touch the nodes they own and leave the rest of
// the document alone. Coordination is process-wide only: a second cozyphi
// process editing the same file is last-writer-wins per section.
package configfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
)

var locks sync.Map // config path → *sync.Mutex

// Edit loads the document at path, hands it to mutate, and — when mutate
// succeeded — commits the edited document atomically. Cycles on the same path
// serialize process-wide from load to commit, and each cycle loads a fresh
// document, so a mutation always sees every change an earlier cycle committed.
// A missing file loads as an empty mapping and is created on commit; an
// unreadable or non-mapping file fails without touching anything on disk.
func Edit(path string, mutate func(doc *yaml.Node) error) error {
	held, _ := locks.LoadOrStore(path, &sync.Mutex{})
	mu := held.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	doc, err := Read(path)
	if err != nil {
		return err
	}
	if err := mutate(doc); err != nil {
		return err
	}
	data, err := Encode(doc)
	if err != nil {
		return err
	}
	// Owner-only: the config carries provider credentials.
	return atomicfile.Write(path, 0o600, data)
}

// Read loads the YAML mapping at path. A missing or empty file yields an empty
// mapping document; any other shape is an error, so a config that cannot be
// re-read is never rewritten. The returned tree is the caller's to inspect —
// persisting changes means going through [Edit].
func Read(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("config %s: read: %w", path, err)
		}
		return EmptyDocument(), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config %s: parse: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return EmptyDocument(), nil
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config %s must be a YAML mapping", path)
	}
	return &doc, nil
}

// EmptyDocument returns a fresh empty mapping document.
func EmptyDocument() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
}

// Lookup returns the node at the key path, or nil when any key along it is
// absent.
func Lookup(doc *yaml.Node, path ...string) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	current := doc.Content[0]
	for _, key := range path {
		if current.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(current.Content); i += 2 {
			if current.Content[i].Value == key {
				next = current.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

// Set installs value at the key path, creating intermediate mappings. A
// non-mapping node in the way of an intermediate key becomes an empty mapping:
// the editor owns the path it edits.
func Set(doc, value *yaml.Node, path ...string) {
	if doc == nil || len(doc.Content) == 0 || len(path) == 0 {
		return
	}
	current := doc.Content[0]
	for i, key := range path {
		last := i == len(path)-1
		var child *yaml.Node
		for j := 0; j+1 < len(current.Content); j += 2 {
			if current.Content[j].Value != key {
				continue
			}
			if last {
				current.Content[j+1] = value
				return
			}
			child = current.Content[j+1]
			break
		}
		if child == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			if last {
				current.Content = append(current.Content, keyNode, value)
				return
			}
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, keyNode, child)
		}
		if child.Kind != yaml.MappingNode {
			child.Kind = yaml.MappingNode
			child.Tag = "!!map"
			child.Content = nil
		}
		current = child
	}
}

// Remove deletes the key at the path. A missing key anywhere along the path is
// a no-op.
func Remove(doc *yaml.Node, path ...string) {
	if doc == nil || len(doc.Content) == 0 || len(path) == 0 {
		return
	}
	current := doc.Content[0]
	for i, key := range path {
		if current.Kind != yaml.MappingNode {
			return
		}
		var next *yaml.Node
		for j := 0; j+1 < len(current.Content); j += 2 {
			if current.Content[j].Value != key {
				continue
			}
			if i == len(path)-1 {
				current.Content = append(current.Content[:j], current.Content[j+2:]...)
				return
			}
			next = current.Content[j+1]
			break
		}
		if next == nil {
			return
		}
		current = next
	}
}

// Token identifies the content of one node for optimistic concurrency: two
// drafts opened against the same token saw the same section. A nil node has
// its own stable token, so "absent" is a comparable state too.
func Token(node *yaml.Node) string {
	var data []byte
	if node != nil {
		data, _ = yaml.Marshal(node)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Encode renders the document as YAML with the house 2-space indent.
func Encode(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("config encode: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("config encode: finish: %w", err)
	}
	return out.Bytes(), nil
}

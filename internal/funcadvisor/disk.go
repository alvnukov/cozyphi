package funcadvisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
	"github.com/alvnukov/cozyphi/internal/funclib"
)

// Three hex digits give a fixed 4096 slots. A collision evicts only derived
// metadata: the Git commit remains available to rebuild either package.
const maxSlotBytes = 32 << 10

type diskRecord struct {
	Name     string            `json:"name"`
	Digest   string            `json:"digest"`
	Checksum [sha256.Size]byte `json:"checksum"`
	Cards    json.RawMessage   `json:"cards"`
}

func slotPath(root, digest string) string {
	if root == "" || (len(digest) != 40 && len(digest) != 64) || strings.ToLower(digest) != digest {
		return ""
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return ""
	}
	return filepath.Join(root, ".git", "cozy-index", "v1", digest[:3]+".json")
}

// IndexRelease records a successful publication for future sessions. The
// library remains usable even if its optional derived cache cannot be written.
func IndexRelease(store *funclib.Store, p funclib.Package) []Card {
	cards := Exports(p)
	if store != nil && p.Version != "" {
		saveCards(store.Root(), p, cards)
	}
	return cards
}

func saveCards(root string, p funclib.Package, cards []Card) {
	path := slotPath(root, p.Digest)
	if path == "" {
		return
	}
	data, err := json.Marshal(cards)
	if err != nil || len(data) > maxSlotBytes {
		return
	}
	record, err := json.Marshal(diskRecord{Name: p.Name, Digest: p.Digest, Checksum: sha256.Sum256(data), Cards: data})
	if err == nil && len(record) <= maxSlotBytes {
		_ = atomicfile.Write(path, 0o600, record)
	}
}

func readCards(root string, ref funclib.Package) ([]Card, bool) {
	path := slotPath(root, ref.Digest)
	if path == "" {
		return nil, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSlotBytes {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close() //nolint:errcheck // read-only descriptor
	data, err := io.ReadAll(io.LimitReader(file, maxSlotBytes+1))
	if err != nil || len(data) > maxSlotBytes {
		return nil, false
	}
	var record diskRecord
	if json.Unmarshal(data, &record) != nil || record.Name != ref.Name || record.Digest != ref.Digest ||
		sha256.Sum256(record.Cards) != record.Checksum {
		return nil, false
	}
	var cards []Card
	if json.Unmarshal(record.Cards, &cards) != nil {
		return nil, false
	}
	for _, c := range cards {
		if c.Draft || c.Version == "" || c.Digest != ref.Digest || c.ID != ref.Name+"/"+c.Name {
			return nil, false
		}
	}
	return cards, true
}

// Lookup searches the checkout's visible releases. Excluding the current
// package finds similar work elsewhere. An empty project searches all latest
// releases so that older checkout pins cannot hide a concurrent publication.
func Lookup(ctx context.Context, store *funclib.Store, project, query, exclude string, limit int) ([]Card, error) {
	var refs []funclib.Package
	var err error
	if project == "" {
		refs, err = store.ReleaseHeads(ctx)
	} else {
		refs, _, err = store.Inventory(ctx, project, "")
	}
	if err != nil {
		return nil, err
	}
	var all []Card
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ref.Name == exclude {
			continue
		}
		cards, err := indexedCards(ctx, store, store.Root(), ref, false)
		if err != nil {
			return nil, err
		}
		all = append(all, cards...)
	}
	return lookup(all, query, limit), nil
}

func indexedCards(
	ctx context.Context,
	store packageSource,
	root string,
	ref funclib.Package,
	draft bool,
) ([]Card, error) {
	if !draft {
		if cards, ok := readCards(root, ref); ok {
			return cards, nil
		}
	}
	p, err := store.Load(ctx, ref.Digest)
	if err != nil {
		return nil, err
	}
	if p.Name != ref.Name || p.Digest != ref.Digest || (p.Version == "") != draft {
		return nil, fmt.Errorf("package %s does not match catalog revision %s", ref.Name, ref.Digest)
	}
	cards := exports(p, draft)
	if !draft {
		saveCards(root, p, cards)
	}
	return cards, nil
}

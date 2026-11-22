package funcadvisor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if json.Unmarshal(data, &record) != nil || record.Name != ref.Name || record.Digest != ref.Digest || sha256.Sum256(record.Cards) != record.Checksum {
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

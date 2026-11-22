package codemode

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"maps"
	"slices"
	"sync"
)

type cachedCell struct {
	key  [32]byte
	cell compiledCell
	size int
}

var bundles = struct {
	sync.Mutex
	byHash map[[32]byte]*list.Element
	recent list.List
	bytes  int
}{byHash: make(map[[32]byte]*list.Element)}

func hashField(h hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.Write([]byte(value))
}

// Hash only compiler inputs. Catalogs, permissions and host handles are bound to
// fresh instances after compiling and can never be retained in the bundle cache.
func loadProgram(p Program, testing bool) (compiledCell, error) {
	if err := validateProgram(p); err != nil {
		return compiledCell{}, err
	}
	h := sha256.New()
	if testing {
		hashField(h, "test")
	} else {
		hashField(h, "run")
	}
	hashField(h, p.Source)
	for _, name := range slices.Sorted(maps.Keys(p.Modules)) {
		hashField(h, name)
		hashField(h, p.Modules[name])
	}
	var key [32]byte
	h.Sum(key[:0])
	bundles.Lock()
	if existing := bundles.byHash[key]; existing != nil {
		bundles.recent.MoveToFront(existing)
		cell := existing.Value.(cachedCell).cell
		bundles.Unlock()
		return cell, nil
	}
	bundles.Unlock()
	cell, err := bundle(p, testing)
	if err != nil {
		return compiledCell{}, err
	}
	size := len(cell.js) + len(cell.mapping.Mappings)
	for _, s := range cell.mapping.Sources {
		size += len(s)
	}
	const capacity = 16 << 20
	if size > capacity {
		return cell, nil
	}
	bundles.Lock()
	defer bundles.Unlock()
	if existing := bundles.byHash[key]; existing != nil {
		return existing.Value.(cachedCell).cell, nil
	}
	bundles.byHash[key] = bundles.recent.PushFront(cachedCell{key, cell, size})
	bundles.bytes += size
	for bundles.bytes > capacity || bundles.recent.Len() > 128 {
		tail := bundles.recent.Back()
		old := bundles.recent.Remove(tail).(cachedCell)
		delete(bundles.byHash, old.key)
		bundles.bytes -= old.size
	}
	return cell, nil
}

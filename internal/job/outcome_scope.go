package job

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Assignment scope is published once, separately from mutable metadata. A broken
// foreign assignment must not prevent another parent's inbox from making progress.
type outcomeScope struct {
	OwnerID  string `json:"owner_id"`
	ParentID string `json:"parent_id"`
}

func (*store) publishScope(meta Meta) error {
	data, err := json.Marshal(outcomeScope{OwnerID: meta.OwnerID, ParentID: meta.ParentID})
	if err != nil {
		return err
	}
	path := filepath.Join(meta.Dir, "scope.json")
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (s *store) outcomeBelongsTo(id, ownerID, parentID string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(s.root, id, "scope.json"))
	if os.IsNotExist(err) {
		// Old jobs have no scope record; incomplete directories cannot identify
		// a subscriber and must not poison every inbox sharing this store.
		meta, readErr := s.readMeta(id)
		if errors.Is(readErr, ErrNotFound) {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
		return meta.OwnerID == ownerID && meta.ParentID == parentID, nil
	}
	if err != nil {
		return false, err
	}
	var scope outcomeScope
	if err := json.Unmarshal(data, &scope); err != nil {
		return false, err
	}
	return scope.OwnerID == ownerID && scope.ParentID == parentID, nil
}

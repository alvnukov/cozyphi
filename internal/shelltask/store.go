package shelltask

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// persist uses an atomic, flushed replacement: a hint never precedes its receipt.
func persist(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode shell task: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".receipt-")
	if err != nil {
		return fmt.Errorf("create shell task receipt: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write shell task receipt: %w", err)
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush shell task receipt: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close shell task receipt: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("publish shell task receipt: %w", err)
	}
	return nil
}

package update_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pulseaiclub/phi/internal/util/update"
)

func TestCheckUsesCacheWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "update-check.json")
	payload, _ := json.Marshal(map[string]any{
		"checked_at": time.Now().UTC(),
		"current_at": "v0.1.0",
		"latest":     "v0.2.0",
		"url":        "https://example.com/releases/tag/v0.2.0",
	})
	if err := os.WriteFile(cache, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	info := update.Check(context.Background(), update.CheckOptions{
		Current:  "v0.1.0",
		CacheDir: dir,
	})
	if !info.Available || info.Latest != "v0.2.0" {
		t.Fatalf("expected cached update, got %+v", info)
	}
}

func TestSkipCheckEnv(t *testing.T) {
	t.Setenv("PHI_SKIP_VERSION_CHECK", "1")
	info := update.Check(context.Background(), update.CheckOptions{Current: "v0.1.0"})
	if info.Available {
		t.Fatalf("expected skip, got %+v", info)
	}
}

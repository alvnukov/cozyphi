package configfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/alvnukov/cozyphi/internal/configfile"
)

// setScalar installs a scalar value at path in doc, the way real editors do.
func setScalar(t *testing.T, doc *yaml.Node, value any, path ...string) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, node.Encode(value))
	configfile.Set(doc, &node, path...)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func TestEditReplacesSectionPreservingUnrelatedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(
		t,
		path,
		"# top comment\nmodels:\n  - name: m\n    api_key: k\nplan:\n  defaults:\n    types: []\ncustom: keep\n",
	)

	var replacement yaml.Node
	require.NoError(t, replacement.Encode(map[string]any{
		"types": []map[string]any{{"name": "review", "tools": []string{"read"}}},
	}))
	err := configfile.Edit(path, func(doc *yaml.Node) error {
		configfile.Set(doc, &replacement, "plan", "defaults")
		return nil
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "# top comment", "comments survive the re-encode")
	assert.Contains(t, string(raw), "custom: keep", "unknown keys survive")
	assert.Contains(t, string(raw), "api_key: k", "untouched sections keep their content")
	assert.Contains(t, string(raw), "name: review", "the replaced section lands on disk")

	doc, err := configfile.Read(path)
	require.NoError(t, err)
	require.NotNil(t, configfile.Lookup(doc, "models"))
}

func TestEditSerializesCyclesAndRereadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "count: 0\n")

	inFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	var secondStarted atomic.Bool
	var seenBySecond atomic.Value
	done := make(chan error, 2)

	go func() {
		done <- configfile.Edit(path, func(doc *yaml.Node) error {
			close(inFirst)
			<-releaseFirst
			setScalar(t, doc, 1, "count")
			return nil
		})
	}()
	<-inFirst
	go func() {
		done <- configfile.Edit(path, func(doc *yaml.Node) error {
			secondStarted.Store(true)
			if node := configfile.Lookup(doc, "count"); node != nil {
				seenBySecond.Store(node.Value)
			}
			setScalar(t, doc, 2, "count")
			return nil
		})
	}()

	// While the first cycle sits in its mutation, the second must not start:
	// its read has to wait for the first commit or it would clobber it.
	deadline := time.Now().Add(200 * time.Millisecond)
	for !secondStarted.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	assert.False(t, secondStarted.Load(), "a second cycle on the same path must wait for the first commit")

	close(releaseFirst)
	require.NoError(t, <-done)
	require.NoError(t, <-done)

	assert.Equal(t, "1", seenBySecond.Load(), "the second cycle loads the first cycle's committed document")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "count: 2")
}

func TestEditMutateErrorLeavesFileUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "models: []\n")

	wantErr := errors.New("reject the draft")
	err := configfile.Edit(path, func(*yaml.Node) error { return wantErr })
	require.ErrorIs(t, err, wantErr)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "models: []\n", string(raw))
}

func TestEditMissingFileCreatesOwnerOnlyConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")

	err := configfile.Edit(path, func(doc *yaml.Node) error {
		setScalar(t, doc, true, "permissions", "dangerously_allow_all")
		return nil
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "dangerously_allow_all: true")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a config created by Edit is owner-only")
}

func TestEditRejectsNonMappingDocument(t *testing.T) {
	for _, body := range []string{"just a scalar\n", "- a\n- b\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		writeFile(t, path, body)

		err := configfile.Edit(path, func(*yaml.Node) error { return nil })
		require.ErrorContains(t, err, "must be a YAML mapping")

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, body, string(raw), "a rejected document is never rewritten")
	}
}

func TestReadMissingFileYieldsEmptyMapping(t *testing.T) {
	doc, err := configfile.Read(filepath.Join(t.TempDir(), "absent.yaml"))
	require.NoError(t, err)
	assert.Nil(t, configfile.Lookup(doc, "anything"), "an empty document has no keys, but reads fine")
}

func TestRemoveDeletesNestedKeyAndToleratesAbsentPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "plan:\n  defaults:\n    types: []\nmodels: []\n")

	err := configfile.Edit(path, func(doc *yaml.Node) error {
		configfile.Remove(doc, "plan", "defaults")
		configfile.Remove(doc, "missing", "key")
		return nil
	})
	require.NoError(t, err)

	doc, err := configfile.Read(path)
	require.NoError(t, err)
	assert.Nil(t, configfile.Lookup(doc, "plan", "defaults"))
	assert.NotNil(t, configfile.Lookup(doc, "models"))
}

func TestRemovePrunesEmptiedParentMappings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "models: []\nagents:\n  models:\n    explore: cheap\n")

	err := configfile.Edit(path, func(doc *yaml.Node) error {
		configfile.Remove(doc, "agents", "models", "explore")
		return nil
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "models: []\n", string(data), "mappings emptied by the removal are pruned up to the root")
}

func TestRemoveKeepsParentWithRemainingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "agents:\n  enabled: false\n  models:\n    explore: cheap\n")

	err := configfile.Edit(path, func(doc *yaml.Node) error {
		configfile.Remove(doc, "agents", "models", "explore")
		return nil
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "agents:\n  enabled: false\n", string(data), "a parent holding other keys survives the prune")
}

func TestTokenDistinguishesSectionVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "plan:\n  defaults:\n    types: []\n")
	first, err := configfile.Read(path)
	require.NoError(t, err)

	before := configfile.Token(configfile.Lookup(first, "plan", "defaults"))
	writeFile(t, path, "plan:\n  defaults:\n    types:\n      - name: review\n")
	second, err := configfile.Read(path)
	require.NoError(t, err)

	assert.NotEqual(t, before, configfile.Token(configfile.Lookup(second, "plan", "defaults")))
	assert.NotPanics(t, func() { configfile.Token(nil) }, "an absent section has a stable token")
}

package voice

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupModelAcceptsEverySpellingOfAName(t *testing.T) {
	for _, name := range []string{"small", "ggml-small", "ggml-small.bin", " Small ", "GGML-SMALL.BIN"} {
		m, ok := LookupModel(name)
		require.True(t, ok, name)
		assert.Equal(t, "small", m.Name, name)
		assert.Equal(t, "ggml-small.bin", m.File, name)
		assert.Contains(t, m.URL, "ggml-small.bin", name)
	}

	_, ok := LookupModel("huge")
	assert.False(t, ok)
	_, ok = LookupModel("")
	assert.False(t, ok)
}

func TestCatalogCoversTheDefaultModelAndCannotBeMutated(t *testing.T) {
	_, ok := LookupModel(DefaultModel)
	assert.True(t, ok)

	got := Catalog()
	require.NotEmpty(t, got)
	got[0].Name = "tampered"
	assert.NotEqual(t, "tampered", Catalog()[0].Name)
}

func TestFormatBytesRendersTheSizesTheCatalogAdvertises(t *testing.T) {
	assert.Equal(t, "466 MB", FormatBytes(466*mib))
	assert.Equal(t, "1.5 GB", FormatBytes(1536*mib))
	assert.Equal(t, "3.1 GB", FormatBytes(3328599654))
	assert.Equal(t, "1.6 GB", FormatBytes(1717986918))
	assert.Equal(t, "512 KB", FormatBytes(512*kib))
	assert.Equal(t, "17 B", FormatBytes(17))
}

func TestInstalledModelsMapsFilesToTheirCatalogModel(t *testing.T) {
	dir := t.TempDir()
	extra := t.TempDir()
	for _, name := range []string{"ggml-medium-q5_0.bin", "ggml-large-v3-turbo-q8_0.bin", "ggml-custom.bin"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("model"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "ggml-dir.bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extra, "ggml-base.bin"), []byte("model"), 0o600))

	got := InstalledModels([]string{dir, extra, "", filepath.Join(dir, "missing")})
	require.Len(t, got, 4)

	byBase := make(map[string]Installed, len(got))
	for _, ins := range got {
		byBase[filepath.Base(ins.Path)] = ins
	}
	assert.Equal(t, "medium", byBase["ggml-medium-q5_0.bin"].Name)
	assert.Equal(t, "large-v3-turbo", byBase["ggml-large-v3-turbo-q8_0.bin"].Name)
	assert.Equal(t, "base", byBase["ggml-base.bin"].Name)
	assert.Equal(t, filepath.Join(extra, "ggml-base.bin"), byBase["ggml-base.bin"].Path)
	assert.Equal(t, int64(5), byBase["ggml-base.bin"].Bytes)

	unknown := byBase["ggml-custom.bin"]
	assert.Empty(t, unknown.Name)
	assert.Equal(t, -1, unknown.Rank)

	assert.Empty(t, InstalledModels(nil))
}

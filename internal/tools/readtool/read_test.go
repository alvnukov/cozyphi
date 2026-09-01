package readtool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunRead_DefaultsToViewOutput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "src/main.go"})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, "@read src/main.go (1 line, 13 bytes, showing 1-2)\n1|package main\n2|\n", out.Content)
	assert.NotContains(t, out.Content, "@file")
	assert.NotContains(t, out.Content, "#")
	assert.Equal(t, "src/main.go (1 line)", out.Detail)
}

func TestRunRead_ViewHeaderCountsRealLines(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("alpha\nbeta\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "f.txt", Offset: 2, Limit: 1})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	// "alpha\nbeta\n" is 2 real lines: the split artifact after a final newline is not one.
	assert.Equal(
		t,
		"@read f.txt (2 lines, 11 bytes, showing 2-2)\n2|beta\n... truncated at 1 lines. Next offset: 3\n",
		out.Content,
	)
}

func TestRunRead_ViewHeaderWithoutRangeWhenPageEmpty(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte("a\nb\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "f.txt", Offset: 9})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, "@read f.txt (2 lines, 4 bytes)\n", out.Content)
}

func TestRunRead_EmptyFileViewCarriesHeader(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "e.txt"), nil, 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "e.txt"})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, "@read e.txt (0 lines, 0 bytes)\n(empty file)", out.Content)
}

func TestRunRead_EditModeReturnsHashlineOutput(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644))
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "src/main.go", Mode: "edit"})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out.Content, "@file src/main.go#"))
	assert.Regexp(t, `(?m)^1#[a-z]{3}\|package main$`, out.Content)
	assert.NotContains(t, out.Content, "@read")
	assert.Equal(t, "src/main.go (1 line)", out.Detail)
}

func TestReadToolSchemaDescribesViewAndEditModes(t *testing.T) {
	tool := ReadTool()
	mode, ok := tool.Definition.Params.Properties["mode"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, []string{"view", "edit"}, mode["enum"])
	require.Contains(t, tool.Definition.Description, `mode:"edit"`)
	require.Contains(t, tool.Definition.Description, "N|content")
	require.Contains(t, tool.Definition.Description, "@read path (N lines")
}

// writeLinesFile fills path with numbered lines until it exceeds minBytes.
func writeLinesFile(t *testing.T, path string, minBytes int) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	w := bufio.NewWriterSize(f, 1<<20)
	for n, written := 1, 0; written <= minBytes; n++ {
		line := fmt.Sprintf("line %d\n", n)
		_, err := w.WriteString(line)
		require.NoError(t, err)
		written += len(line)
	}
	require.NoError(t, w.Flush())
}

func TestRunRead_LargeFileViewIsWindowed(t *testing.T) {
	root := t.TempDir()
	writeLinesFile(t, filepath.Join(root, "big.log"), readMaxHashBytes)
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "big.log", Offset: 3, Limit: 2})
	require.NoError(t, err)
	out, err := runRead(t.Context(), raw)
	require.NoError(t, err)
	assert.Regexp(t, `^@read big\.log \([0-9.]+ (KiB|MiB), showing 3-4\)\n`, out.Content)
	_, page, _ := strings.Cut(out.Content, "\n")
	assert.Equal(t, "3|line 3\n4|line 4\n... truncated at 2 lines. Next offset: 5\n", page)
	assert.Equal(t, "big.log (8.0 MiB)", out.Detail)
}

func TestRunRead_LargeFileEditModeStillRefused(t *testing.T) {
	root := t.TempDir()
	writeLinesFile(t, filepath.Join(root, "big.log"), readMaxHashBytes)
	t.Chdir(root)

	raw, err := json.Marshal(readInput{Path: "big.log", Mode: "edit"})
	require.NoError(t, err)
	_, err = runRead(t.Context(), raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refuse to hash files larger than")
}

// The windowed path is only reached for large files, so its rendering is
// pinned against the in-memory path it stands in for.
func TestReadViewWindow_MatchesInMemoryRendering(t *testing.T) {
	for name, content := range map[string]string{
		"trailing newline": "alpha\nbeta\n",
		"no final newline": "alpha\nbeta",
		"crlf":             "alpha\r\nbeta\r\n",
		"blank lines":      "alpha\n\n\nbeta\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "f.txt"), []byte(content), 0o644))
			t.Chdir(root)

			raw, err := json.Marshal(readInput{Path: "f.txt"})
			require.NoError(t, err)
			inMemory, err := runRead(t.Context(), raw)
			require.NoError(t, err)

			// The header is composed by runRead, not by the windowed renderer;
			// drop it so the comparison pins page rendering only.
			_, page, _ := strings.Cut(inMemory.Content, "\n")
			windowed, _, err := readViewWindow(t.Context(), "f.txt", 1, readDefaultMaxLines)
			require.NoError(t, err)
			assert.Equal(t, page, windowed)
		})
	}
}

func TestReadLineBounded_DropsWhatDoesNotFit(t *testing.T) {
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", 5000)+"\nnext\n"), 64)

	line, eof, err := readLineBounded(reader, 10)
	require.NoError(t, err)
	assert.False(t, eof)
	assert.Equal(t, strings.Repeat("x", 10), line, "an over-long line is kept only up to the cap")

	line, _, err = readLineBounded(reader, 10)
	require.NoError(t, err)
	assert.Equal(t, "next", line, "the reader resumes at the next line, not mid-line")
}

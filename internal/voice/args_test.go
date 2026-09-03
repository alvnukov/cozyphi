package voice

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitArgsHandlesQuotesAndEscapes(t *testing.T) {
	tests := map[string]struct {
		line string
		want []string
	}{
		"plain":         {`whisper-cli -m model.bin`, []string{"whisper-cli", "-m", "model.bin"}},
		"double quotes": {`ffmpeg -i "My Mic"`, []string{"ffmpeg", "-i", "My Mic"}},
		"single quotes": {`sh -c 'echo hi'`, []string{"sh", "-c", "echo hi"}},
		"escape":        {`echo a\ b`, []string{"echo", "a b"}},
		"empty arg":     {`echo "" tail`, []string{"echo", "", "tail"}},
		"extra spaces":  {"  a   b  ", []string{"a", "b"}},
		"empty line":    {"   ", nil},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := splitArgs(tc.line)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSplitArgsRejectsAnUnbalancedQuote(t *testing.T) {
	_, err := splitArgs(`ffmpeg -i "My Mic`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unbalanced")
}

func TestExpandArgsSubstitutesPlaceholders(t *testing.T) {
	argv := []string{"whisper-cli", "-m", "{model}", "-l", "{lang}", "-f", "{file}"}
	got := expandArgs(argv, map[string]string{
		"model": "ggml-base.bin",
		"lang":  "ru",
		"file":  "/tmp/a.wav",
	})
	assert.Equal(t, []string{"whisper-cli", "-m", "ggml-base.bin", "-l", "ru", "-f", "/tmp/a.wav"}, got)
}

func TestExpandArgsDropsAnEmptyPlaceholderWithItsFlag(t *testing.T) {
	argv := []string{"whisper-cli", "--prompt", "{hint}", "-nt", "-f", "{file}"}
	got := expandArgs(argv, map[string]string{"hint": "", "file": "/tmp/a.wav"})
	assert.Equal(t, []string{"whisper-cli", "-nt", "-f", "/tmp/a.wav"}, got)
}

func TestExpandArgsKeepsAPlaceholderEmbeddedInAWord(t *testing.T) {
	got := expandArgs([]string{"--model={model}"}, map[string]string{"model": "m.bin"})
	assert.Equal(t, []string{"--model=m.bin"}, got)
}

func TestSolePlaceholderRecognizesOnlyAWholeArgument(t *testing.T) {
	name, ok := solePlaceholder("{file}")
	assert.True(t, ok)
	assert.Equal(t, "file", name)

	_, ok = solePlaceholder("--f={file}")
	assert.False(t, ok)

	_, ok = solePlaceholder("{}")
	assert.False(t, ok)
}

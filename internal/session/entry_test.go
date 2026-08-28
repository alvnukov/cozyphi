package session

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailsFileLists(t *testing.T) {
	details := CompactionDetails{ReadFiles: []string{"a.go"}, ModifiedFiles: []string{"b.go"}}

	t.Run("in-process payload", func(t *testing.T) {
		read, modified := DetailsFileLists(details)
		assert.Equal(t, []string{"a.go"}, read)
		assert.Equal(t, []string{"b.go"}, modified)
	})

	t.Run("decoded session file payload", func(t *testing.T) {
		raw, err := json.Marshal(details)
		require.NoError(t, err)
		var reloaded map[string]any
		require.NoError(t, json.Unmarshal(raw, &reloaded))
		read, modified := DetailsFileLists(reloaded)
		assert.Equal(t, []string{"a.go"}, read)
		assert.Equal(t, []string{"b.go"}, modified)
	})

	t.Run("absent or foreign payload", func(t *testing.T) {
		read, modified := DetailsFileLists(nil)
		assert.Empty(t, read)
		assert.Empty(t, modified)
		read, modified = DetailsFileLists("other")
		assert.Empty(t, read)
		assert.Empty(t, modified)
	})
}

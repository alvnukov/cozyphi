package chat

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveSlashArg(t *testing.T) {
	cases := []struct {
		name        string
		value       string
		cursor      int
		wantName    string
		wantPartial string
		wantStart   int
		wantEnd     int
		wantOK      bool
	}{
		{"first arg at end", "/theme op", 9, "theme", "op", 7, 9, true},
		{"cursor inside arg", "/theme op", 8, "theme", "o", 7, 9, true},
		{"space just typed", "/theme ", 7, "theme", "", 7, 7, true},
		{"extra spaces skipped", "/theme   op", 11, "theme", "op", 9, 11, true},
		{"still editing name", "/theme", 6, "", "", 0, 0, false},
		{"cursor before the space", "/theme ", 6, "", "", 0, 0, false},
		{"second argument", "/export a b", 11, "", "", 0, 0, false},
		{"bare slash", "/ ", 2, "", "", 0, 0, false},
		{"no leading slash", "theme op", 8, "", "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, partial, start, end, ok := ActiveSlashArg(tc.value, tc.cursor)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantName, name)
				assert.Equal(t, tc.wantPartial, partial)
				assert.Equal(t, tc.wantStart, start)
				assert.Equal(t, tc.wantEnd, end)
			}
		})
	}
}

// TestNotifyCompletersFiresSlashArg pins the wiring: editing into the first
// argument of a leading command reaches OnSlashArgChange with the command
// name and the partial argument.
func TestNotifyCompletersFiresSlashArg(t *testing.T) {
	var gotActive bool
	var gotName, gotPartial string
	c := ChatInput{
		Value: "/theme op",
		OnSlashArgChange: func(active bool, name, partial string) {
			gotActive, gotName, gotPartial = active, name, partial
		},
	}
	c.Cursor = 8
	c.notifyCompleters()

	assert.True(t, gotActive)
	assert.Equal(t, "theme", gotName)
	assert.Equal(t, "o", gotPartial)
}

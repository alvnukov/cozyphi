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
		wantArgs    []string
		wantPartial string
		wantStart   int
		wantEnd     int
		wantOK      bool
	}{
		{"first arg at end", "/theme op", 9, "theme", nil, "op", 7, 9, true},
		{"cursor inside arg", "/theme op", 8, "theme", nil, "o", 7, 9, true},
		{"space just typed", "/theme ", 7, "theme", nil, "", 7, 7, true},
		{"extra spaces skipped", "/theme   op", 11, "theme", nil, "op", 9, 11, true},
		{"still editing name", "/theme", 6, "", nil, "", 0, 0, false},
		{"cursor before the space", "/theme ", 6, "", nil, "", 0, 0, false},
		{"second argument", "/voice install sm", 17, "voice", []string{"install"}, "sm", 15, 17, true},
		{"second argument space typed", "/voice install ", 15, "voice", []string{"install"}, "", 15, 15, true},
		{"back in the first argument", "/voice install x", 10, "voice", nil, "ins", 7, 14, true},
		{"bare slash", "/ ", 2, "", nil, "", 0, 0, false},
		{"no leading slash", "theme op", 8, "", nil, "", 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, args, partial, start, end, ok := ActiveSlashArg(tc.value, tc.cursor)
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantName, name)
				assert.Equal(t, tc.wantArgs, args)
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
	var gotArgs []string
	c := ChatInput{
		Value: "/theme op",
		OnSlashArgChange: func(active bool, name string, args []string, partial string) {
			gotActive, gotName, gotArgs, gotPartial = active, name, args, partial
		},
	}
	c.Cursor = 8
	c.notifyCompleters()

	assert.True(t, gotActive)
	assert.Equal(t, "theme", gotName)
	assert.Empty(t, gotArgs)
	assert.Equal(t, "o", gotPartial)
}

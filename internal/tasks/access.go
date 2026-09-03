package tasks

import (
	"fmt"
	"strings"
)

// Access is how far a session may go with the registry. It is the user's
// call, read from permissions.tasks: the registry is theirs, and whether the
// model keeps it, proposes each change, or only reads it is a matter of how
// they like to work, not of safety — a note is one file under a fixed
// directory, never code.
type Access string

const (
	// AccessOff hides the registry: no task tool, no word about it in the
	// system prompt.
	AccessOff Access = "off"
	// AccessRead offers current, list and get; every write is refused with
	// the advice to describe the change for the user.
	AccessRead Access = "read"
	// AccessAsk offers every action and asks the user before each write,
	// in plan mode too — the user chose to decide, and is there to.
	AccessAsk Access = "ask"
	// AccessWrite lets the model keep the registry on its own, the way it
	// writes memory. The default.
	AccessWrite Access = "write"
)

// Accesses lists the levels from least to most trusting, for menus and
// error hints.
var Accesses = []Access{AccessOff, AccessRead, AccessAsk, AccessWrite}

// ParseAccess reads a config value. Empty means the default; anything else
// must be one of Accesses, and the error names them.
func ParseAccess(raw string) (Access, error) {
	value := Access(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return AccessWrite, nil
	}
	for _, level := range Accesses {
		if value == level {
			return level, nil
		}
	}
	return "", fmt.Errorf("permissions.tasks: unknown value %q (use off, read, ask or write)", raw)
}

// Normalized returns the level with the empty default filled in, so a policy
// built without ParseAccess still reads as write.
func (a Access) Normalized() Access {
	if a == "" {
		return AccessWrite
	}
	return a
}

// Writable reports whether write actions are offered at all: ask and write
// are, read and off are not.
func (a Access) Writable() bool {
	switch a.Normalized() {
	case AccessAsk, AccessWrite:
		return true
	default:
		return false
	}
}

// Next steps to the following level in the settings row. It tightens from
// the default — write, ask, read, off — because that is what a person
// reaching for the row usually wants, and wraps back to write.
func (a Access) Next() Access {
	switch a.Normalized() {
	case AccessWrite:
		return AccessAsk
	case AccessAsk:
		return AccessRead
	case AccessRead:
		return AccessOff
	default:
		return AccessWrite
	}
}

func (a Access) String() string { return string(a.Normalized()) }

package diag

// KeybindConfigFacts is what the configuration says about the keyboard: the
// vocabulary of rebindable commands this build understands, and which of them
// the keybinds section names. It is answered in every process shape — a run
// that renders nothing still has a configuration somebody wrote.
//
// It is an allowlist by construction: a chord spelling is a config value that
// nothing here needs, so there is no member for one to land in. Which keys a
// command answers to is a question for the help screen, which is where a
// person can see it without a tool being asked.
type KeybindConfigFacts struct {
	// Known is false when nobody published the configuration.
	Known bool
	// Commands lists every rebindable command id, in dispatch order. It is
	// the vocabulary a keybinds section may name and is fixed by the build.
	Commands []string
	// Overrides lists the command ids the keybinds section names, sorted.
	Overrides []string
}

// KeybindRuntimeFacts is the compiled binding table's own account of itself,
// published by the goroutine that owns it. The table is process-wide: one
// dialect and one set of chords serve every session on screen.
type KeybindRuntimeFacts struct {
	// Known is false when the surface published no table.
	Known bool
	// Profile is the editing dialect the table in force was compiled for.
	Profile string
	// Editing is the dialect the composer is editing in. It is the same as
	// Profile in a session nothing went wrong in, and the pair is reported
	// separately precisely so a session where they came apart can say so.
	Editing string
	// Accepted lists the command ids the compiled table took an override
	// for, sorted. A configured id the compiler refused never reaches it.
	Accepted []string
	// Diverged lists the command ids whose chords differ from what this
	// profile's own defaults would be, sorted. A profile switch moves the
	// defaults, so a command can be configured and undiverged at once.
	Diverged []string
}

// Sources for the keyboard's layers.
var (
	sourceKeymapStored = Source{
		Kind: SourceConfigFile,
		Ref:  "the editing dialect the stored UI preferences persist, read once where this session was built",
	}
	sourceKeymapUnstored = Source{
		Kind: SourceConfigFile,
		Ref:  "nothing persisted an editing dialect, so the composer starts in the one it defaults to",
	}
	sourceKeymapUnread = Source{
		Kind: SourceUnknown,
		Ref: "the stored UI preferences were not read for this session, so what they persist is not " +
			"something this view may go and find out",
	}
	sourceKeymapProfile = Source{
		Kind: SourceSession,
		Ref: "the dialect the binding table in force was compiled for; one table serves every session of " +
			"this process",
	}
	sourceKeymapEditing = Source{
		Kind: SourceSession,
		Ref:  "the dialect the composer is editing in right now",
	}
	sourceKeybindsVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the rebindable commands this build understands; a keybinds entry naming anything else fails the load",
	}
	sourceKeybindsConfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "the command ids the configuration's keybinds section names; the chords it gives them are not reported",
	}
	sourceKeybindsUnconfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "the configuration rebinds nothing, so every command answers to the chord it was built with",
	}
	sourceKeybindsAccepted = Source{
		Kind: SourceSession,
		Ref: "the command ids the compiled table took an override for; the table is built once at boot and " +
			"a rebind that failed to compile never reached it",
	}
	sourceKeybindsDiverged = Source{
		Kind: SourceSession,
		Ref: "the command ids whose chords differ from this dialect's own defaults; switching dialects moves " +
			"the defaults, so a rebound command can end up answering to what it would have anyway",
	}
	sourceKeyboardUnpublished = Source{
		Kind: SourceUnknown,
		Ref:  "the surface has not published its binding table yet",
	}
)

// keymapMode is the editing dialect three questions deep: what the
// preferences persist, what the binding table in force was compiled for, and
// what the composer is editing in. The last two are one answer in a session
// that is working.
func (s UISurfaceFacts) keymapMode(config UIConfigFacts) Field {
	field := s.field(KeyKeymapMode, ApplyImmediate, ScopeProcess)
	switch {
	case !config.Known:
	case !config.KeymapRead:
		field.Configured = unknown(sourceKeymapUnread)
	case config.Keymap == "":
		field.Configured = Unset(NoValue(), sourceKeymapUnstored)
	default:
		field.Configured = Present(StringValue(config.Keymap), sourceKeymapStored)
	}
	field.Loaded = s.runtime(func() Observation {
		if !s.Keys.Known {
			return unknown(sourceKeyboardUnpublished)
		}
		return Present(StringValue(s.Keys.Profile), sourceKeymapProfile)
	})
	field.Effective = s.runtime(func() Observation {
		if !s.Keys.Known {
			return unknown(sourceKeyboardUnpublished)
		}
		return Present(StringValue(s.Keys.Editing), sourceKeymapEditing)
	})
	return field
}

// keybindCommands is the vocabulary a keybinds section may name. It is a
// build constant, and it is answered in a process that renders nothing too:
// what may be configured does not stop being true because nothing is painting.
func (s UISurfaceFacts) keybindCommands(config UIConfigFacts) Field {
	field := s.field(KeyKeybindsCommands, ApplyRestart, ScopeProcess)
	field.Configured = notApplicable(sourceKeybindsVocabulary)
	if !config.Known || !config.Keybinds.Known {
		return field
	}
	field.Loaded = Present(ListValue(config.Keybinds.Commands), sourceKeybindsVocabulary)
	field.Effective = field.Loaded
	return field
}

// keybindOverrides is what the configuration rebinds, what the compiled table
// took from it, and what actually ends up spelled differently from the
// dialect's own defaults. Names only: which chord a command answers to is not
// something a tool has to be told.
func (s UISurfaceFacts) keybindOverrides(config UIConfigFacts) Field {
	field := s.field(KeyKeybindsOverrides, ApplyRestart, ScopeProcess)
	switch {
	case !config.Known || !config.Keybinds.Known:
	case len(config.Keybinds.Overrides) == 0:
		field.Configured = Unset(ListValue(nil), sourceKeybindsUnconfigured)
	default:
		field.Configured = Present(ListValue(config.Keybinds.Overrides), sourceKeybindsConfigured)
	}
	field.Loaded = s.runtime(func() Observation {
		if !s.Keys.Known {
			return unknown(sourceKeyboardUnpublished)
		}
		return Present(ListValue(s.Keys.Accepted), sourceKeybindsAccepted)
	})
	field.Effective = s.runtime(func() Observation {
		if !s.Keys.Known {
			return unknown(sourceKeyboardUnpublished)
		}
		return Present(ListValue(s.Keys.Diverged), sourceKeybindsDiverged)
	})
	return field
}

package diag

// NotifyConfigFacts is the notifications section as its owner decoded it. It
// is answered in every process shape: a run that renders nothing still has a
// mode and a sound somebody set, and reporting them as absent would hide a
// setting rather than describe a process.
type NotifyConfigFacts struct {
	// Known is false when nobody published the section.
	Known bool
	// Mode is when a notification would fire: off, always or unfocused.
	Mode string
	// Sound is what one would play. Empty means silence, which is a setting
	// rather than a missing value.
	Sound string
}

// NotifierFacts is the live notifier's account of itself. Only what decides
// whether a ping happens travels: not a title, not a body, not the detail a
// question put in one, and not the error a failed sender returned.
type NotifierFacts struct {
	// Known is false when no notifier is attached to this surface.
	Known bool
	// Mode is the mode the notifier holds now. The settings pane changes it
	// mid-session without the configuration being read again, which is
	// exactly when it stops matching the configured layer.
	Mode string
	// Sound is what the next notification would ask for; empty is silence.
	Sound string
	// Broken is whether the platform sender failed once and switched
	// notifications off for the rest of the process.
	Broken bool
	// FocusTrusted is whether the terminal has ever reported losing focus.
	// Until it has, "focused" may just be the optimistic assumption every
	// session starts with, and the unfocused mode keeps notifying.
	FocusTrusted bool
	// Focused is whether the terminal window is believed to have focus.
	Focused bool
}

// NotifyDelivery is what a notification would actually do right now — the
// mode, the focus and the sender's health added up. It is the answer to the
// question people actually ask, which is never "what is the mode".
type NotifyDelivery string

// NotifyDelivery values.
const (
	// NotifyDeliveryArmed means a notification now would reach the user.
	NotifyDeliveryArmed NotifyDelivery = "armed"
	// NotifyDeliverySuppressed means the mode is unfocused and the terminal
	// is believed to be focused, so a notification would be dropped.
	NotifyDeliverySuppressed NotifyDelivery = "suppressed"
	// NotifyDeliveryOff means the mode switches notifications off.
	NotifyDeliveryOff NotifyDelivery = "off"
	// NotifyDeliveryBroken means the platform sender failed and the notifier
	// stopped trying for the rest of the process.
	NotifyDeliveryBroken NotifyDelivery = "broken"
	// NotifyDeliveryUnattached means no notifier is attached at all.
	NotifyDeliveryUnattached NotifyDelivery = "unattached"
)

// The two mode spellings this package has to recognize on their own: a
// broken sender amounts to "off" whatever the notifier still says, and
// "unfocused" is the only mode the terminal's focus can suppress.
const (
	notifyModeOff       = "off"
	notifyModeUnfocused = "unfocused"
)

// Sources for the notifier's layers.
var (
	sourceNotifyConfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "the notifications.mode the configuration was loaded with",
	}
	sourceNotifyLive = Source{
		Kind: SourceSession,
		Ref: "the mode the notifier holds now; the settings pane applies a committed change straight to it, " +
			"so this leads the configured layer until the configuration is read again",
	}
	sourceNotifyGated = Source{
		Kind: SourceComputed,
		Ref:  "what that mode amounts to now: a sender that already failed leaves notifications off whatever it says",
	}
	sourceNotifySoundConfigured = Source{
		Kind: SourceConfigFile,
		Ref:  "the notifications.sound the configuration was loaded with",
	}
	sourceNotifySoundSilent = Source{
		Kind: SourceConfigFile,
		Ref:  "notifications.sound is off, so a notification arrives silently",
	}
	sourceNotifySoundLive = Source{
		Kind: SourceSession,
		Ref:  "the sound the next notification would ask the platform for",
	}
	sourceNotifySoundMuted = Source{
		Kind: SourceSession,
		Ref:  "the notifier holds no sound, so a notification would arrive silently",
	}
	sourceNotifyOutcome = Source{
		Kind: SourceComputed,
		Ref: "nothing configures the outcome on its own: it is the mode, the terminal's focus and the " +
			"sender's health added up",
	}
	sourceNotifyAttached = Source{
		Kind: SourceSession,
		Ref:  "whether this surface has a notifier at all; without one nothing is sent and nothing failed",
	}
	sourceNotifyFocusTerminal = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures focus: the terminal reports it, and some terminals never do",
	}
	sourceNotifyFocusTrust = Source{
		Kind: SourceSession,
		Ref: "whether the terminal has ever reported losing focus; until it has, focus is assumed rather " +
			"than known and the unfocused mode keeps notifying",
	}
	sourceNotifyFocusLive = Source{
		Kind: SourceSession,
		Ref:  "whether the terminal window is believed to have focus right now",
	}
	sourceNotifierUnattached = Source{
		Kind: SourceSession,
		Ref:  "no notifier is attached to this surface, so there is no mode, no sound and no failure to report",
	}
)

// notifyMode is when a notification fires: what the configuration was loaded
// with, what the notifier holds now, and what that amounts to after a sender
// that already failed is taken into account.
func (s UISurfaceFacts) notifyMode(config UIConfigFacts) Field {
	field := s.field(KeyNotifyMode, ApplyImmediate, ScopeSession)
	if config.Known && config.Notifications.Known {
		field.Configured = Present(StringValue(config.Notifications.Mode), sourceNotifyConfigured)
	}
	field.Loaded = s.notifier(func() Observation {
		return Present(StringValue(s.Notifications.Mode), sourceNotifyLive)
	})
	field.Effective = s.notifier(func() Observation {
		return Present(StringValue(s.effectiveNotifyMode()), sourceNotifyGated)
	})
	return field
}

// notifySound is what a notification would play. Silence is a setting, so it
// is reported as one rather than as a value nobody set.
func (s UISurfaceFacts) notifySound(config UIConfigFacts) Field {
	field := s.field(KeyNotifySound, ApplyImmediate, ScopeSession)
	switch {
	case !config.Known || !config.Notifications.Known:
	case config.Notifications.Sound == "":
		field.Configured = Unset(NoValue(), sourceNotifySoundSilent)
	default:
		field.Configured = Present(StringValue(config.Notifications.Sound), sourceNotifySoundConfigured)
	}
	field.Loaded = s.notifier(func() Observation {
		if s.Notifications.Sound == "" {
			return Unset(NoValue(), sourceNotifySoundMuted)
		}
		return Present(StringValue(s.Notifications.Sound), sourceNotifySoundLive)
	})
	field.Effective = field.Loaded
	return field
}

// notifyDelivery is whether a ping would reach the user right now. It is the
// question people actually ask, and no single layer below answers it.
func (s UISurfaceFacts) notifyDelivery() Field {
	field := s.field(KeyNotifyDelivery, ApplyImmediate, ScopeSession)
	field.Configured = notApplicable(sourceNotifyOutcome)
	field.Loaded = s.runtime(func() Observation {
		return Present(BoolValue(s.Notifications.Known), sourceNotifyAttached)
	})
	field.Effective = s.runtime(func() Observation {
		return Present(StringValue(string(s.delivery())), sourceNotifyOutcome)
	})
	return field
}

// notifyFocus is what the unfocused mode turns on. A terminal that never
// reports focus is the case worth seeing: the notifier assumes focus, does
// not trust the assumption, and keeps notifying because of it.
func (s UISurfaceFacts) notifyFocus() Field {
	field := s.field(KeyNotifyFocus, ApplyImmediate, ScopeSession)
	field.Configured = notApplicable(sourceNotifyFocusTerminal)
	field.Loaded = s.notifier(func() Observation {
		return Present(BoolValue(s.Notifications.FocusTrusted), sourceNotifyFocusTrust)
	})
	field.Effective = s.notifier(func() Observation {
		return Present(BoolValue(s.Notifications.Focused), sourceNotifyFocusLive)
	})
	return field
}

// notifier is runtime narrowed by one more absence: a surface may render and
// still have no notifier attached, and that is a state of its own rather than
// a mode nobody set.
func (s UISurfaceFacts) notifier(observe func() Observation) Observation {
	return s.runtime(func() Observation {
		if !s.Notifications.Known {
			return Unset(NoValue(), sourceNotifierUnattached)
		}
		return observe()
	})
}

// effectiveNotifyMode is the mode after a sender that already failed: the
// notifier stops trying for the rest of the process, whatever the mode says.
func (s UISurfaceFacts) effectiveNotifyMode() string {
	if s.Notifications.Broken {
		return notifyModeOff
	}
	return s.Notifications.Mode
}

// delivery adds the mode, the focus and the sender's health up into the one
// answer that says whether a notification now would arrive.
func (s UISurfaceFacts) delivery() NotifyDelivery {
	switch {
	case !s.Notifications.Known:
		return NotifyDeliveryUnattached
	case s.Notifications.Broken:
		return NotifyDeliveryBroken
	case s.Notifications.Mode == notifyModeOff:
		return NotifyDeliveryOff
	case s.Notifications.Mode == notifyModeUnfocused && s.Notifications.FocusTrusted && s.Notifications.Focused:
		return NotifyDeliverySuppressed
	default:
		return NotifyDeliveryArmed
	}
}

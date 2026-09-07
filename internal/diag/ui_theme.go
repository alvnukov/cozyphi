package diag

// ThemeFacts is what the harness may say about the palette a surface paints
// with: which one it started under, which one is in force now, and which ones
// this build carries at all.
//
// It is an allowlist by construction: a palette is a table of colors and
// attributes, and none of it travels — only the name the picker knows it by.
type ThemeFacts struct {
	// Known is false when the surface published no palette — a surface that
	// has not finished building rather than one painting in no colors.
	Known bool
	// Builtin lists the palette names this build carries, in picker order.
	Builtin []string
	// Boot is the palette the surface was built with.
	Boot string
	// Live is the palette in force now. It differs from Boot exactly when
	// somebody ran /theme, and it survives no restart: nothing persists it.
	Live string
}

// Sources for the palette's layers.
var (
	sourceThemeUnpersisted = Source{
		Kind: SourceDefault,
		Ref: "no setting persists a palette: a surface starts under the house default and /theme changes " +
			"it for this session only",
	}
	sourceThemeBoot = Source{
		Kind: SourceDefault,
		Ref:  "the palette this surface was built with",
	}
	sourceThemeLive = Source{
		Kind: SourceSession,
		Ref:  "the palette painting now; it is the boot one until somebody picks another",
	}
	sourceThemeBuild = Source{
		Kind: SourceBuild,
		Ref:  "the palettes compiled into this build; no setting adds one",
	}
	sourceThemeUnbuilt = Source{
		Kind: SourceUnknown,
		Ref:  "the surface has not published a palette yet",
	}
)

// themeName is the palette three questions deep: what persists it, what the
// surface started under, and what is painting now. The first answer is the
// interesting one — nothing persists a palette, so a session that looks wrong
// after a restart was never configured to look any other way.
func (s UISurfaceFacts) themeName() Field {
	field := s.field(KeyThemeName, ApplyImmediate, ScopeSession)
	field.Configured = Unset(NoValue(), sourceThemeUnpersisted)
	field.Loaded = s.runtime(func() Observation {
		if !s.Theme.Known {
			return unknown(sourceThemeUnbuilt)
		}
		return Present(StringValue(s.Theme.Boot), sourceThemeBoot)
	})
	field.Effective = s.runtime(func() Observation {
		if !s.Theme.Known {
			return unknown(sourceThemeUnbuilt)
		}
		return Present(StringValue(s.Theme.Live), sourceThemeLive)
	})
	return field
}

// themeBuiltin is what /theme has to choose from. It is a build constant, so
// no source asked for it — and a process that renders nothing loads no
// palette at all, which is the honest answer rather than a list of names
// nothing could paint with.
func (s UISurfaceFacts) themeBuiltin() Field {
	field := s.field(KeyThemeBuiltin, ApplyRestart, ScopeProcess)
	field.Configured = notApplicable(sourceThemeBuild)
	field.Loaded = s.runtime(func() Observation {
		if !s.Theme.Known {
			return unknown(sourceThemeUnbuilt)
		}
		return Present(ListValue(s.Theme.Builtin), sourceThemeBuild)
	})
	field.Effective = field.Loaded
	return field
}

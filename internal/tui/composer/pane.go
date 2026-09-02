package composer

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/clipboard"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/chat"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/pathutil"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
	"github.com/alvnukov/cozyphi/internal/util/filesearch"
)

// ComposerPane owns the chat input, slash/@ pickers, and palette.
type ComposerPane struct {
	theme components.Theme
	cwd   string

	Chat    chat.ChatInput
	mention mention.Picker
	slash   mention.Picker
	palette palette.CommandPalette

	mentionGen int
	commands   *commands.CommandRegistry
	// paletteRefresh rebuilds the root command list on every Ctrl+K open, so
	// the palette never replays a stale snapshot. The editor installs it; the
	// composer does not know how to build a CommandContext.
	paletteRefresh func() []palette.PaletteCommand
	mode           agent.Mode
	bashActive     bool

	// slashArgMode is true while the slash picker lists argument values
	// instead of command names; accept then replaces the argument token.
	slashArgMode bool
	transcript   *transcript.TranscriptPane
	submitter    BusyChecker

	// history records submissions so Up/Down in the composer can recall
	// them; nil degrades to plain caret movement.
	history *history.Store

	bus SubmitBus

	focus Focuser

	// attachedMedia holds an inline image pasted into the composer; it is
	// carried on submit so the model receives it alongside the text.
	attachedMedia []llm.Media

	// readClipboard reads an image from the system clipboard; it is a seam so
	// tests can stub the platform read.
	readClipboard func() (clipboard.Image, bool, error)
}

// NewComposerPane builds composer widgets; call Wire before use. hist may be
// nil — the composer then works without prompt history.
func NewComposerPane(theme components.Theme, model, cwd string, hist *history.Store) *ComposerPane {
	c := &ComposerPane{
		theme: theme,
		cwd:   cwd,
		Chat:  newChatInput(theme, model, cwd),
		mention: mention.Picker{
			Theme: theme,
		},
		slash: mention.Picker{
			Theme:  theme,
			Prefix: "/",
		},
		palette: palette.CommandPalette{
			Theme: theme,
		},
		history:       hist,
		readClipboard: clipboard.ReadImage,
	}
	if hist != nil {
		c.Chat.History = hist
	}
	return c
}

// Wire binds bus, transcript, and focus after Editor assembly. It is the
// second half of construction by necessity: Overlays, Submitter, and
// HookCommands take the composer before its collaborators exist, so the
// transcript/bus/focus binding can only happen once assembly is done.
// Overlay-vs-composer focus arbitration is not a parameter — it lives in the
// Focuser adapter (Editor.Focus keeps focus at the root while a modal owns
// the keyboard).
func (c *ComposerPane) Wire(
	transcript *transcript.TranscriptPane,
	submitter BusyChecker,
	commands *commands.CommandRegistry,
	cwd string,
	bus SubmitBus,
	focus Focuser,
) {
	if c == nil {
		return
	}
	c.cwd = cwd
	c.commands = commands
	c.transcript = transcript
	c.submitter = submitter
	c.bus = bus
	c.focus = focus

	c.palette.FocusReturn = &c.Chat
	c.Chat.OnSubmit = func(text string) {
		c.history.Append(text)
		if c.bus != nil {
			c.bus.Publish(controller.SubmitMsg{Text: text, Media: c.attachedMedia})
			c.bus.DrainNow()
		}
		c.attachedMedia = nil
		c.Chat.HintsRight = nil
	}
	c.Chat.OnChange = func(text string) {
		c.SyncBashBorder(text)
		if c.bus != nil {
			c.bus.RequestRefresh()
		}
	}
	c.Chat.OnMentionChange = c.onMentionChange
	c.Chat.OnSlashChange = c.onSlashChange
	c.Chat.OnSlashArgChange = c.onSlashArgChange
	c.mention.OnAccept = c.acceptMention
	c.slash.OnAccept = c.acceptSlash
}

// AttachMedia appends an inline image to the composer and shows it in the
// hints row. It replaces any previously attached image.
func (c *ComposerPane) AttachMedia(media llm.Media) {
	if c == nil {
		return
	}
	c.attachedMedia = []llm.Media{media}
	c.Chat.HintsRight = []components.Span{{Text: "📷 " + media.MediaType}}
}

// ClearAttachedMedia removes the attached image, restoring the hint fallback.
func (c *ComposerPane) ClearAttachedMedia() {
	if c == nil {
		return
	}
	c.attachedMedia = nil
	c.Chat.HintsRight = nil
}

// AttachedMedia returns the currently attached image, if any.
func (c *ComposerPane) AttachedMedia() []llm.Media {
	if c == nil {
		return nil
	}
	return c.attachedMedia
}

// pasteImage attaches a clipboard image when the system clipboard holds one.
// It returns true when an image was attached (so the text paste is skipped);
// otherwise the composer falls back to pasting text as usual.
func (c *ComposerPane) pasteImage() bool {
	img, ok, err := c.readClipboard()
	if err != nil || !ok {
		return false
	}
	c.AttachMedia(llm.Media{MediaType: img.MediaType, Data: base64.StdEncoding.EncodeToString(img.Data)})
	return true
}

// HideCompleters closes mention and slash pickers.
func (c *ComposerPane) HideCompleters() {
	if c == nil {
		return
	}
	c.mention.Hide()
	c.Chat.MentionOpen = false
	c.mentionGen++
	c.slash.Hide()
	c.Chat.SlashOpen = false
	c.slashArgMode = false
}

// HidePalette closes the command palette if open.
func (c *ComposerPane) HidePalette() {
	if c != nil {
		c.palette.Hide()
	}
}

// ClearInput clears the chat composer text and drops any history walk in
// progress, so the next Up starts from the newest entry.
func (c *ComposerPane) ClearInput() {
	if c == nil {
		return
	}
	c.Chat.Value = ""
	c.Chat.Cursor = 0
	c.Chat.ClearSelection()
	c.history.Reset()
}

// SetChatCopyFunc wires the composer's clipboard callback (copy/cut chords).
func (c *ComposerPane) SetChatCopyFunc(fn func(text string) bool) {
	if c != nil {
		c.Chat.OnCopy = fn
	}
}

// PendingSkills returns attached skill names awaiting submit.
func (c *ComposerPane) PendingSkills() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.Chat.PendingSkills))
	out = append(out, c.Chat.PendingSkills...)
	return out
}

// ClearPendingSkills removes attached skills from the composer.
func (c *ComposerPane) ClearPendingSkills() {
	if c != nil {
		c.Chat.ClearPendingSkills()
	}
}

// SyncBashBorder updates composer chrome for "!cmd" prefix.
func (c *ComposerPane) SyncBashBorder(text string) {
	if c != nil && c.submitter != nil {
		c.submitter.SyncBashBorder(text)
	}
}

// SetBashBorderActive recolors the posture bar while a "!cmd" prefix is active.
func (c *ComposerPane) SetBashBorderActive(active bool) {
	if c == nil {
		return
	}
	c.bashActive = active
	c.applyPosture()
}

// FocusChat requests keyboard focus on the chat input.
func (c *ComposerPane) FocusChat() {
	if c != nil && c.focus != nil {
		c.focus.Focus(&c.Chat)
	}
}

// AddPendingSkill attaches a skill badge to the composer.
func (c *ComposerPane) AddPendingSkill(name string) {
	if c != nil {
		c.Chat.AddPendingSkill(name)
	}
}

// SetMode switches the posture lead between build, plan, and useplan.
func (c *ComposerPane) SetMode(m agent.Mode) {
	if c == nil {
		return
	}
	c.mode = m
	c.applyPosture()
}

// applyPosture paints the posture lead, its bar color, and the matching
// placeholder: build → Secondary, plan → Warning, useplan → Violet, bash
// prefix → ToolName.
func (c *ComposerPane) applyPosture() {
	text := "⏵⏵ useplan"
	style := c.theme.Violet
	switch c.mode {
	case agent.ModeBuild:
		text = "⏵⏵ build"
		style = c.theme.Secondary
	case agent.ModePlan:
		text = "⏵⏵ plan"
		style = c.theme.Warning
	}
	placeholder := askPlaceholder
	if c.bashActive {
		style = c.theme.ToolName
		placeholder = shellPlaceholder
	}
	c.Chat.AgentLabel = layout.BorderLabel{Text: text, Style: style}
	c.Chat.Placeholder = placeholder
}

// SetModelLabel updates the model name in the composer meta row.
func (c *ComposerPane) SetModelLabel(name string) {
	if c != nil {
		c.Chat.ModelLabel = name
	}
}

// SetBranchLabel updates the cwd path on the composer hints row.
func (c *ComposerPane) SetBranchLabel(text string) {
	if c != nil {
		c.Chat.HintsLeft = text
	}
}

// ClearUsageHints clears token/context stats; the keymap fallback returns.
func (c *ComposerPane) ClearUsageHints() {
	if c != nil {
		c.Chat.HintsRight = nil
	}
}

// SetUsageHints sets token/context spans on the composer hints row.
func (c *ComposerPane) SetUsageHints(spans []components.Span) {
	if c != nil {
		c.Chat.HintsRight = spans
	}
}

// SetPaletteCommands replaces Ctrl+K root commands.
func (c *ComposerPane) SetPaletteCommands(cmds []palette.PaletteCommand) {
	if c != nil {
		c.palette.Commands = cmds
	}
}

// SetPaletteRefresh installs the supplier called on every Ctrl+K open, so the
// root list is rebuilt (and re-ranked) instead of replaying a stale snapshot.
func (c *ComposerPane) SetPaletteRefresh(refresh func() []palette.PaletteCommand) {
	if c != nil {
		c.paletteRefresh = refresh
	}
}

// PushPalette opens or nests a palette submenu.
func (c *ComposerPane) PushPalette(title string, cmds []palette.PaletteCommand) {
	if c == nil {
		return
	}
	if !c.palette.Open {
		c.palette.Show()
	}
	c.palette.Push(title, cmds)
	if c.focus != nil {
		c.focus.Focus(&c.palette)
	}
}

// SetTheme updates composer widget themes.
func (c *ComposerPane) SetTheme(th components.Theme) {
	if c == nil {
		return
	}
	c.theme = th
	c.Chat.Theme = th
	c.Chat.TextStyle = th.Foreground
	c.applyPosture()
	c.palette.Theme = th
	c.mention.Theme = th
	c.slash.Theme = th
	c.SyncBashBorder(c.Chat.Value)
}

// ApplyMentionResults updates the @ picker from async file search. Agent
// roles are re-merged on top: the async replace must not drop them.
func (c *ComposerPane) ApplyMentionResults(msg controller.MentionResultsMsg) {
	if c == nil || msg.Gen != c.mentionGen || !c.mention.Open {
		return
	}
	if msg.ErrText != "" {
		c.mention.SetResults(matchingAgentMentions(msg.Query), msg.ErrText)
		return
	}
	items := make([]mention.Item, 0, len(msg.Paths)+3)
	items = append(items, matchingAgentMentions(msg.Query)...)
	for _, p := range msg.Paths {
		items = append(items, mention.Item{Path: p})
	}
	status := ""
	if len(items) == 0 {
		status = "No matching files"
	}
	c.mention.SetResults(items, status)
}

// PreferredHeight reports the chat input area height; ChatInput floors it
// at MinHeight itself, so no caller re-derives the floor.
func (c *ComposerPane) PreferredHeight(width int, method xui.WidthMethod) int {
	if c == nil {
		return c.MinHeight()
	}
	return c.Chat.PreferredHeight(width, method)
}

// MinHeight is the composer's smallest height (frame floor incl. pending
// skills); the editor clamps short screens against it.
func (c *ComposerPane) MinHeight() int {
	if c == nil {
		return 8 // zero-config ChatInput: MinBodyRows 3 + frame chrome 5
	}
	return c.Chat.MinHeight()
}

// DrawChat renders the chat input surface.
func (c *ComposerPane) DrawChat(ctx components.DrawContext, width, height int) components.Surface {
	if c == nil {
		return components.Surface{}
	}
	return c.Chat.Draw(
		ctx.WithConstraints(components.Size{}, components.Size{Width: width, Height: height}),
	)
}

// PickerOverlays returns slash and @ picker surfaces anchored above the composer.
func (c *ComposerPane) PickerOverlays(ctx components.DrawContext, listH, width int) []components.SubSurface {
	if c == nil {
		return nil
	}
	var out []components.SubSurface
	if c.slash.Open {
		c.slash.AnchorBottomY = listH
		c.slash.AnchorX = 0
		c.slash.AnchorWidth = width
		out = append(out, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: c.slash.Draw(ctx),
			Z:       components.ZPicker,
		})
	}
	if c.mention.Open {
		c.mention.AnchorBottomY = listH
		c.mention.AnchorX = 0
		c.mention.AnchorWidth = width
		out = append(out, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: c.mention.Draw(ctx),
			Z:       components.ZPicker,
		})
	}
	return out
}

// PaletteOverlay returns the Ctrl+K palette surface when open.
func (c *ComposerPane) PaletteOverlay(ctx components.DrawContext) (components.SubSurface, bool) {
	if c == nil || !c.palette.Open {
		return components.SubSurface{}, false
	}
	return components.SubSurface{
		Origin:  components.Point{X: 0, Y: 0},
		Surface: c.palette.Draw(ctx),
		Z:       components.ZPalette,
	}, true
}

// Handle dispatches keyboard/mouse input to the composer area.
func (c *ComposerPane) Handle(ctx *components.EventContext, ev xui.Event) {
	if c == nil {
		return
	}
	switch ev := ev.(type) {
	case xui.FocusEvent:
		if c.palette.Open {
			if c.focus != nil {
				c.focus.Focus(&c.palette)
			}
		} else {
			c.FocusChat()
		}
	case xui.KeyEvent:
		// Plain Ctrl+C without a composer selection never arrives here (the
		// App runtime hands it to the root as an interrupt, and quits on the
		// second one, before dispatch), so controller cleanup is owned by
		// Run's caller (cmd); a claimed copy chord stops in ChatInput.
		if ev.Press && ev.Code == xui.KeyEscape {
			if c.slash.Open {
				c.slash.Cancel()
				c.Chat.SlashOpen = false
				ctx.ConsumeAndRedraw()
				return
			}
			if c.mention.Open {
				c.mention.Cancel()
				c.Chat.MentionOpen = false
				c.mentionGen++
				ctx.ConsumeAndRedraw()
				return
			}
			if c.submitter != nil && !c.submitter.CanSubmit() {
				if c.bus != nil {
					c.bus.Publish(controller.CancelStreamMsg{})
					c.bus.DrainNow()
				}
				ctx.ConsumeAndRedraw()
				return
			}
			if c.transcript != nil && c.transcript.SelectionActive() {
				c.transcript.ClearSelection()
				ctx.ConsumeAndRedraw()
				return
			}
		}
		if ev.Press && ev.Code == xui.KeyTab &&
			!c.slash.Open && !c.mention.Open && !c.palette.Open {
			if c.bus != nil {
				c.bus.Publish(controller.ModeToggleMsg{})
				c.bus.DrainNow()
			}
			ctx.ConsumeAndRedraw()
			return
		}
		// The palette chord resolves through the keys table so a keybinds
		// override reaches it; the action stays here because the palette
		// is the composer's widget.
		if keys.Is(ev, keys.CmdPalette) {
			if c.palette.Open {
				c.palette.Hide()
				c.FocusChat()
			} else {
				c.HideCompleters()
				if c.paletteRefresh != nil {
					c.palette.Commands = c.paletteRefresh()
				}
				c.palette.Show()
				if c.focus != nil {
					c.focus.Focus(&c.palette)
				}
			}
			ctx.ConsumeAndRedraw()
			return
		}
		// Ctrl+V attaches a clipboard image (mirrors opencode's prompt.paste
		// keybind): terminals do not send a bracketed-paste event for an
		// image, so we read the clipboard ourselves. When the clipboard holds
		// no image we fall through so text paste still works.
		if ev.Press && ev.Code == xui.KeyRune && ev.Mods.Has(xui.ModCtrl) &&
			ev.HotkeyRune() == 'v' {
			if c.pasteImage() {
				c.HideCompleters()
				ctx.ConsumeAndRedraw()
				return
			}
		}
		if c.palette.Open {
			if ctx.DeliveredTo != &c.palette {
				c.palette.Handle(ctx, ev)
			}
			if !c.palette.Open {
				c.FocusChat()
			}
			return
		}
		if c.slash.Open && mentionNavKey(ev) {
			c.slash.Handle(ctx, ev)
			if !c.slash.Open {
				c.Chat.SlashOpen = false
			}
			return
		}
		if c.mention.Open && mentionNavKey(ev) {
			c.mention.Handle(ctx, ev)
			if !c.mention.Open {
				c.Chat.MentionOpen = false
			}
			return
		}
		if ev.Code == xui.KeyPageUp || ev.Code == xui.KeyPageDown {
			if c.transcript != nil {
				c.transcript.HandlePageKey(ctx, ev)
			}
			return
		}
		if len(c.attachedMedia) > 0 && ev.Press && ev.Mods.Has(xui.ModAlt) &&
			ev.Code == xui.KeyRune && ev.HotkeyRune() == 'x' {
			c.ClearAttachedMedia()
			ctx.ConsumeAndRedraw()
			return
		}
		c.maybeChatHandle(ctx, ev)
	case xui.MouseEvent:
		if c.palette.Open {
			if ctx.DeliveredTo != &c.palette {
				c.palette.Handle(ctx, ev)
			}
			return
		}
		if c.transcript != nil {
			c.transcript.HandleMouse(ctx, ev, c.FocusChat)
		}
	case xui.PasteEvent:
		if c.palette.Open {
			if ctx.DeliveredTo != &c.palette {
				c.palette.Handle(ctx, ev)
			}
			return
		}
		if c.pasteImage() {
			ctx.ConsumeAndRedraw()
			return
		}
		c.maybeChatHandle(ctx, ev)
	}
}

// maybeChatHandle forwards to the chat input unless the dispatcher already
// delivered this event to it (focused delivery before the root ladder): the
// chat just refused the key, so re-running its Handle could mutate twice.
func (c *ComposerPane) maybeChatHandle(ctx *components.EventContext, ev xui.Event) {
	if ctx.DeliveredTo != &c.Chat {
		c.Chat.Handle(ctx, ev)
	}
}

func (c *ComposerPane) onMentionChange(active bool, query string) {
	if c == nil {
		return
	}
	if !active {
		c.mention.Hide()
		c.Chat.MentionOpen = false
		c.mentionGen++
		return
	}
	if c.slash.Open || c.Chat.SlashOpen {
		return
	}
	c.slash.Hide()
	c.Chat.SlashOpen = false
	c.mention.Show()
	c.Chat.MentionOpen = true
	c.mention.SetResults(matchingAgentMentions(query), "")
	if len(c.mention.Items) == 0 {
		c.mention.Status = "Searching…"
	}
	c.scheduleMentionSearch(query)
}

func (c *ComposerPane) onSlashChange(active bool, query string) {
	if c == nil {
		return
	}
	if !active {
		c.slash.Hide()
		c.Chat.SlashOpen = false
		return
	}
	// The cursor is in the command name: name mode owns the picker.
	c.slashArgMode = false
	c.mention.Hide()
	c.Chat.MentionOpen = false
	c.mentionGen++
	items := []mention.Item{}
	if c.commands != nil {
		items = c.commands.FilterSlash(query)
	}
	status := ""
	if len(items) == 0 {
		status = "No matching commands"
	}
	c.slash.SetResults(items, status)
	c.slash.Show()
	c.Chat.SlashOpen = true
}

// onSlashArgChange routes the picker into argument mode: the command token
// is complete and the cursor sits in its first argument, so the picker
// lists that command's argument values instead of command names.
func (c *ComposerPane) onSlashArgChange(active bool, name, partial string) {
	if c == nil {
		return
	}
	if !active {
		if c.slashArgMode {
			c.slash.Hide()
			c.Chat.SlashOpen = false
			c.slashArgMode = false
		}
		return
	}
	if c.commands == nil {
		return
	}
	items, ok := c.commands.CompleteSlashArg(name, partial)
	if !ok {
		return
	}
	c.mention.Hide()
	c.Chat.MentionOpen = false
	c.slashArgMode = true
	status := ""
	if len(items) == 0 {
		status = "No matching values"
	}
	c.slash.SetResults(items, status)
	c.slash.Show()
	c.Chat.SlashOpen = true
}

func (c *ComposerPane) scheduleMentionSearch(query string) {
	if c == nil {
		return
	}
	c.mentionGen++
	gen := c.mentionGen
	cwd := c.cwd
	bus := c.bus
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		paths, err := filesearch.Search(ctx, cwd, query, 20)
		msg := controller.MentionResultsMsg{Gen: gen, Query: query, Paths: paths}
		if err != nil {
			msg.ErrText = err.Error()
		}
		if bus != nil {
			bus.Publish(msg)
		}
	}()
}

func (c *ComposerPane) acceptMention(item mention.Item) {
	if c == nil {
		return
	}
	_, start, end, ok := chat.ActiveMention(c.Chat.Value, c.Chat.Cursor)
	if !ok {
		start, end = c.Chat.Cursor, c.Chat.Cursor
	}
	c.mentionGen++
	c.mention.Hide()
	c.Chat.MentionOpen = false
	c.Chat.ReplaceRange(start, end, "@"+item.Path+" ")
}

func (c *ComposerPane) acceptSlash(item mention.Item) {
	if c == nil {
		return
	}
	if c.slashArgMode {
		// Argument mode: replace the argument token, keep the command.
		if _, _, start, end, ok := chat.ActiveSlashArg(c.Chat.Value, c.Chat.Cursor); ok {
			c.Chat.ReplaceRange(start, end, item.Path+" ")
		}
		c.slashArgMode = false
		c.slash.Hide()
		c.Chat.SlashOpen = false
		return
	}
	_, start, end, ok := chat.ActiveSlash(c.Chat.Value, c.Chat.Cursor)
	if !ok {
		start, end = 0, c.Chat.Cursor
	}
	insert := ""
	if c.commands != nil {
		insert = c.commands.LookupInsert(item.Path)
	}
	if insert == "" {
		insert = "/" + item.Path
	}
	c.Chat.ReplaceRange(start, end, insert)
	c.slash.Hide()
	c.Chat.SlashOpen = false
	if !strings.HasSuffix(insert, " ") {
		if c.bus != nil {
			c.bus.Publish(controller.SubmitMsg{Text: strings.TrimSpace(insert)})
			c.bus.DrainNow()
		}
	}
}

func newChatInput(theme components.Theme, model, cwd string) chat.ChatInput {
	return chat.ChatInput{
		MinBodyRows:    1,
		MaxBodyRows:    8,
		UseBlockCursor: false,
		Theme:          theme,
		TextStyle:      theme.Foreground,
		CursorStyle:    xui.Style{Reverse: true},
		AgentLabel:     layout.BorderLabel{Text: "⏵⏵ useplan", Style: theme.Violet},
		ModelLabel:     model,
		HintsLeft:      pathutil.PathWithBranch(cwd),
		Placeholder:    askPlaceholder,
	}
}

// Composer placeholders mirror opencode's prompt: a question hint in ask
// posture, a command hint while a "!" prefix switches to shell mode.
const (
	askPlaceholder   = "Ask anything..."
	shellPlaceholder = "Run a command..."
)

// agentMentions are the @-picker sub-agent roles; names mirror job roles the
// engine's delegation parser accepts (leading "@role " in a prompt).
var agentMentions = []mention.Item{
	{Path: string(job.RoleExplore), Description: "read-only codebase search", Agent: true},
	{Path: string(job.RoleReview), Description: "read-only diffs and checks", Agent: true},
	{Path: string(job.RoleWorker), Description: "implements a scoped change", Agent: true},
}

// matchingAgentMentions returns agent roles whose name starts with query
// (case-insensitive). Roles sit above file results in the picker.
func matchingAgentMentions(query string) []mention.Item {
	q := strings.ToLower(query)
	var out []mention.Item
	for _, it := range agentMentions {
		if q == "" || strings.HasPrefix(strings.ToLower(it.Path), q) {
			out = append(out, it)
		}
	}
	return out
}

func mentionNavKey(e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	switch e.Code {
	case xui.KeyUp, xui.KeyDown, xui.KeyTab, xui.KeyEnter, xui.KeyEscape:
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) &&
			(e.HotkeyRune() == 'n' || e.HotkeyRune() == 'N' || e.HotkeyRune() == 'p' || e.HotkeyRune() == 'P') {
			return true
		}
	}
	return false
}

// Ensure ComposerPane implements Input.
var _ Input = (*ComposerPane)(nil)

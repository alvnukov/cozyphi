package composer

import (
	"context"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/chat"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/footer"
	"github.com/pulseaiclub/phi/internal/tui/pathutil"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
	"github.com/pulseaiclub/phi/internal/util/filesearch"
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

	transcript *transcript.TranscriptPane
	submitter  BusyChecker

	publish  func(controller.Msg)
	drainBus func()
	onRedraw func()

	overlayBlocksComposer func() bool
	handlePermissionKey   WireKeyHandler
	handleContinueKey     WireKeyHandler
	handleCopyKey         WireKeyHandler
	requestFocusEditor    func()
	requestFocus          func(components.Widget)
}

// NewComposerPane builds composer widgets; call Wire before use.
func NewComposerPane(theme components.Theme, model, cwd string) *ComposerPane {
	return &ComposerPane{
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
	}
}

// Wire binds bus, transcript, and editor overlay hooks after Editor assembly.
func (c *ComposerPane) Wire(
	transcript *transcript.TranscriptPane,
	submitter BusyChecker,
	commands *commands.CommandRegistry,
	cwd string,
	publish func(controller.Msg),
	drainBus func(),
	onRedraw func(),
	overlayBlocksComposer func() bool,
	handlePermissionKey WireKeyHandler,
	handleContinueKey WireKeyHandler,
	handleCopyKey WireKeyHandler,
	requestFocusEditor func(),
	requestFocus func(components.Widget),
) {
	if c == nil {
		return
	}
	c.cwd = cwd
	c.commands = commands
	c.transcript = transcript
	c.submitter = submitter
	c.publish = publish
	c.drainBus = drainBus
	c.onRedraw = onRedraw
	c.overlayBlocksComposer = overlayBlocksComposer
	c.handlePermissionKey = handlePermissionKey
	c.handleContinueKey = handleContinueKey
	c.handleCopyKey = handleCopyKey
	c.requestFocusEditor = requestFocusEditor
	c.requestFocus = requestFocus

	c.palette.FocusReturn = &c.Chat
	c.Chat.OnSubmit = func(text string) {
		if c.publish != nil {
			c.publish(controller.SubmitMsg{Text: text})
		}
		if c.drainBus != nil {
			c.drainBus()
		}
	}
	c.Chat.OnChange = func(text string) {
		c.SyncBashBorder(text)
		if c.onRedraw != nil {
			c.onRedraw()
		}
	}
	c.Chat.OnMentionChange = c.onMentionChange
	c.Chat.OnSlashChange = c.onSlashChange
	c.mention.OnAccept = c.acceptMention
	c.slash.OnAccept = c.acceptSlash
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
}

// HidePalette closes the command palette if open.
func (c *ComposerPane) HidePalette() {
	if c != nil {
		c.palette.Hide()
	}
}

// ClearInput clears the chat composer text.
func (c *ComposerPane) ClearInput() {
	if c == nil {
		return
	}
	c.Chat.Value = ""
	c.Chat.Cursor = 0
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

// CloseMentionSlash hides @ and / pickers.
func (c *ComposerPane) CloseMentionSlash() {
	if c == nil {
		return
	}
	c.mention.Hide()
	c.Chat.MentionOpen = false
	c.mentionGen++
	c.slash.Hide()
	c.Chat.SlashOpen = false
}

// SetBashBorderActive toggles bash-mode border styling.
func (c *ComposerPane) SetBashBorderActive(active bool) {
	if c == nil {
		return
	}
	if active {
		c.Chat.BorderStyle = c.theme.ToolName
	} else {
		c.Chat.BorderStyle = c.theme.Border
	}
}

// FocusChat requests keyboard focus on the chat input.
func (c *ComposerPane) FocusChat() {
	if c != nil && c.requestFocus != nil {
		c.requestFocus(&c.Chat)
	}
}

// AddPendingSkill attaches a skill badge to the composer.
func (c *ComposerPane) AddPendingSkill(name string) {
	if c != nil {
		c.Chat.AddPendingSkill(name)
	}
}

// SetModelLabel updates the model name in the composer header.
func (c *ComposerPane) SetModelLabel(name string) {
	if c != nil {
		c.Chat.TopRightLabel.Text = name
	}
}

// SetBranchLabel updates the path label in the composer footer.
func (c *ComposerPane) SetBranchLabel(text string) {
	if c != nil {
		c.Chat.BottomRightLabel.Text = text
	}
}

// ClearBottomLeftLabel clears token/context stats in the composer footer.
func (c *ComposerPane) ClearBottomLeftLabel() {
	if c != nil {
		c.Chat.BottomLeftLabel = layout.BorderLabel{}
	}
}

// SetBottomLeftLabel sets token/context stats in the composer footer.
func (c *ComposerPane) SetBottomLeftLabel(label layout.BorderLabel) {
	if c != nil {
		c.Chat.BottomLeftLabel = label
	}
}

// SetPaletteCommands replaces Ctrl+K root commands.
func (c *ComposerPane) SetPaletteCommands(cmds []palette.PaletteCommand) {
	if c != nil {
		c.palette.Commands = cmds
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
	if c.requestFocus != nil {
		c.requestFocus(&c.palette)
	}
}

// SetTheme updates composer widget themes.
func (c *ComposerPane) SetTheme(th components.Theme) {
	if c == nil {
		return
	}
	c.theme = th
	c.Chat.Theme = th
	c.Chat.BorderStyle = th.Border
	c.Chat.TextStyle = th.Foreground
	c.Chat.BottomRightLabel.Style = footer.PathLabelStyle(th)
	c.Chat.TopRightLabel.Style = th.Success
	c.palette.Theme = th
	c.mention.Theme = th
	c.slash.Theme = th
	c.SyncBashBorder(c.Chat.Value)
}

// ApplyMentionResults updates the @ picker from async file search.
func (c *ComposerPane) ApplyMentionResults(msg controller.MentionResultsMsg) {
	if c == nil || msg.Gen != c.mentionGen || !c.mention.Open {
		return
	}
	if msg.ErrText != "" {
		c.mention.SetResults(nil, msg.ErrText)
		return
	}
	items := make([]mention.Item, 0, len(msg.Paths))
	for _, p := range msg.Paths {
		items = append(items, mention.Item{Path: p})
	}
	status := ""
	if len(items) == 0 {
		status = "No matching files"
	}
	c.mention.SetResults(items, status)
}

// PreferredHeight reports the chat input area height.
func (c *ComposerPane) PreferredHeight(width int, method xui.WidthMethod) int {
	if c == nil {
		return 5
	}
	chatH := c.Chat.PreferredHeight(width, method)
	minChatH := 5
	if len(c.Chat.PendingSkills) > 0 {
		minChatH++
	}
	if chatH < minChatH {
		chatH = minChatH
	}
	return chatH
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
			Z:       15,
		})
	}
	if c.mention.Open {
		c.mention.AnchorBottomY = listH
		c.mention.AnchorX = 0
		c.mention.AnchorWidth = width
		out = append(out, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: c.mention.Draw(ctx),
			Z:       15,
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
		Z:       20,
	}, true
}

// Handle dispatches keyboard/mouse input to the composer area.
func (c *ComposerPane) Handle(ctx *components.EventContext, ev xui.Event) {
	if c == nil {
		return
	}
	switch ev := ev.(type) {
	case xui.FocusEvent:
		if c.overlayBlocksComposer != nil && c.overlayBlocksComposer() {
			if c.requestFocusEditor != nil {
				c.requestFocusEditor()
			}
		} else if c.palette.Open {
			if c.requestFocus != nil {
				c.requestFocus(&c.palette)
			}
		} else {
			c.FocusChat()
		}
	case xui.KeyEvent:
		// Ctrl+C never arrives here: the App runtime quits on it before
		// dispatch, so controller cleanup is owned by Run's caller (cmd).
		if c.handlePermissionKey != nil && c.handlePermissionKey(ctx, ev) {
			return
		}
		if c.handleContinueKey != nil && c.handleContinueKey(ctx, ev) {
			return
		}
		if c.handleCopyKey != nil && c.handleCopyKey(ctx, ev) {
			return
		}
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
			if c.submitter != nil && (c.submitter.RunningBash() || c.submitter.IsBusy()) {
				if c.publish != nil {
					c.publish(controller.CancelStreamMsg{})
				}
				if c.drainBus != nil {
					c.drainBus()
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
		if ev.Press && ev.Mods.Has(xui.ModCtrl) && ev.Code == xui.KeyRune &&
			(ev.Rune == 'k' || ev.Rune == 'K') {
			if c.palette.Open {
				c.palette.Hide()
				c.FocusChat()
			} else {
				c.HideCompleters()
				c.palette.Show()
				if c.requestFocus != nil {
					c.requestFocus(&c.palette)
				}
			}
			ctx.ConsumeAndRedraw()
			return
		}
		if c.palette.Open {
			c.palette.Handle(ctx, ev)
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
		c.Chat.Handle(ctx, ev)
	case xui.MouseEvent:
		if c.palette.Open {
			c.palette.Handle(ctx, ev)
			return
		}
		if c.transcript != nil {
			c.transcript.HandleMouse(ctx, ev, c.FocusChat)
		}
	case xui.PasteEvent:
		if c.palette.Open {
			c.palette.Handle(ctx, ev)
			return
		}
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

func (c *ComposerPane) scheduleMentionSearch(query string) {
	if c == nil {
		return
	}
	c.mentionGen++
	gen := c.mentionGen
	cwd := c.cwd
	publish := c.publish
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		paths, err := filesearch.Search(ctx, cwd, query, 20)
		msg := controller.MentionResultsMsg{Gen: gen, Query: query, Paths: paths}
		if err != nil {
			msg.ErrText = err.Error()
		}
		if publish != nil {
			publish(msg)
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
		if c.publish != nil {
			c.publish(controller.SubmitMsg{Text: strings.TrimSpace(insert)})
		}
		if c.drainBus != nil {
			c.drainBus()
		}
	}
}

func newChatInput(theme components.Theme, model, cwd string) chat.ChatInput {
	return chat.ChatInput{
		MinBodyRows:    3,
		MaxBodyRows:    8,
		UseBlockCursor: false,
		PaddingX:       1,
		Theme:          theme,
		BorderStyle:    theme.Border,
		TextStyle:      theme.Foreground,
		CursorStyle:    xui.Style{Reverse: true},
		TopRightLabel: layout.BorderLabel{
			Text:  model,
			Style: theme.Success,
		},
		BottomRightLabel: layout.BorderLabel{
			Text:  pathutil.PathWithBranch(cwd),
			Style: footer.PathLabelStyle(theme),
		},
	}
}

func mentionNavKey(e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	switch e.Code {
	case xui.KeyUp, xui.KeyDown, xui.KeyTab, xui.KeyEnter, xui.KeyEscape:
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) && (e.Rune == 'n' || e.Rune == 'N' || e.Rune == 'p' || e.Rune == 'P') {
			return true
		}
	}
	return false
}

// Ensure ComposerPane implements Input.
var _ Input = (*ComposerPane)(nil)

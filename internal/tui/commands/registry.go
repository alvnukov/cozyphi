package commands

import (
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
)

// Host is the capability surface commands reach to drive the editor shell.
// *editor.Editor implements it; tests implement a fake. Commands never hold
// *Editor, keeping the package free of the root widget.
type Host interface {
	Toast(msg string, kind toast.ToastKind, d time.Duration)
	PushSubmenu(title string, cmds []palette.PaletteCommand)

	ShowSessions()
	ResumeSession(id string)
	ClearSession() // may toast internally if busy

	SetModel(name string)
	ApplyTheme(name string)
	SetPermissions(bypass bool)
	SetAgents(enabled bool)
	ReloadHooks()
	ListHooks() []palette.PaletteCommand
	AddSkill(name string)
	CopyLastMessage()

	ModelNames() []string
	SkillPath() string
}

// CommandContext is the capability surface passed to command Run / palette
// builders. Args carries slash arguments; Host is the editor shell.
type CommandContext struct {
	Args []string // slash args after the command name

	Host Host
}

func (ctx CommandContext) toast(msg string, kind toast.ToastKind, d time.Duration) {
	if ctx.Host != nil {
		ctx.Host.Toast(msg, kind, d)
	}
}

// Command is one registered slash and/or palette entry.
type Command struct {
	Name        string // without leading slash, e.g. "resume"
	Description string
	Slash       bool
	// Insert is written into the composer on slash-picker accept.
	// Empty defaults to "/"+Name.
	Insert string

	// Run handles slash dispatch (and may be unused for palette-only trees).
	Run func(ctx CommandContext) error

	// PaletteRoot builds a Ctrl+K root row when non-nil.
	PaletteRoot func(ctx CommandContext) palette.PaletteCommand

	fromHook bool // dropped on hooks reload; cannot replace builtins
}

// CommandRegistry is the single catalog for composer `/` and Ctrl+K palette.
type CommandRegistry struct {
	mu   sync.RWMutex
	cmds []Command
	by   map[string]int // lower(name) → index in cmds
}

// NewCommandRegistry returns an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{by: make(map[string]int)}
}

// Register adds cmd. Duplicate names (case-insensitive) replace the prior entry.
func (r *CommandRegistry) Register(cmd Command) {
	if r == nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(cmd.Name))
	if name == "" {
		return
	}
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.Slash && cmd.Insert == "" {
		cmd.Insert = "/" + cmd.Name
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.by == nil {
		r.by = make(map[string]int)
	}
	if i, ok := r.by[name]; ok {
		r.cmds[i] = cmd
		return
	}
	r.by[name] = len(r.cmds)
	r.cmds = append(r.cmds, cmd)
}

// registerHook adds a slash command from a KindCommand hook.
// Returns false if name is empty or already taken by a builtin.
func (r *CommandRegistry) registerHook(cmd Command) bool {
	if r == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(cmd.Name))
	if name == "" {
		return false
	}
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.fromHook = true
	if cmd.Slash && cmd.Insert == "" {
		cmd.Insert = "/" + cmd.Name
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.by == nil {
		r.by = make(map[string]int)
	}
	if i, ok := r.by[name]; ok {
		if !r.cmds[i].fromHook {
			return false
		}
		r.cmds[i] = cmd
		return true
	}
	r.by[name] = len(r.cmds)
	r.cmds = append(r.cmds, cmd)
	return true
}

// clearHookCommands removes every command registered via registerHook.
func (r *CommandRegistry) clearHookCommands() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := make([]Command, 0, len(r.cmds))
	r.by = make(map[string]int, len(r.cmds))
	for _, c := range r.cmds {
		if c.fromHook {
			continue
		}
		r.by[strings.ToLower(c.Name)] = len(kept)
		kept = append(kept, c)
	}
	r.cmds = kept
}

// DispatchSlash runs a `/name …` line. Returns false if not a known slash command.
func (r *CommandRegistry) DispatchSlash(text string, ctx CommandContext) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	name := strings.TrimPrefix(fields[0], "/")
	cmd, ok := r.lookup(name)
	if !ok || !cmd.Slash || cmd.Run == nil {
		return false
	}
	ctx.Args = fields[1:]
	_ = cmd.Run(ctx)
	return true
}

// FilterSlash returns mention items for the slash picker (name prefix match).
func (r *CommandRegistry) FilterSlash(query string) []mention.Item {
	q := strings.ToLower(strings.TrimSpace(query))
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mention.Item, 0, len(r.cmds))
	for _, c := range r.cmds {
		if !c.Slash {
			continue
		}
		if q != "" && !strings.HasPrefix(strings.ToLower(c.Name), q) {
			continue
		}
		out = append(out, mention.Item{
			Path:        c.Name,
			Description: c.Description,
		})
	}
	return out
}

// LookupInsert returns the Insert string for a slash command name, or empty.
func (r *CommandRegistry) LookupInsert(name string) string {
	cmd, ok := r.lookup(name)
	if !ok || !cmd.Slash {
		return ""
	}
	return cmd.Insert
}

// BuildPalette returns Ctrl+K root commands in registration order.
func (r *CommandRegistry) BuildPalette(ctx CommandContext) []palette.PaletteCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]palette.PaletteCommand, 0, len(r.cmds))
	for _, c := range r.cmds {
		if c.PaletteRoot == nil {
			continue
		}
		out = append(out, c.PaletteRoot(ctx))
	}
	return out
}

func (r *CommandRegistry) lookup(name string) (Command, bool) {
	if r == nil {
		return Command{}, false
	}
	key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "/"))
	r.mu.RLock()
	defer r.mu.RUnlock()
	i, ok := r.by[key]
	if !ok {
		return Command{}, false
	}
	return r.cmds[i], true
}

// SlashCommands returns slash catalog entries (for tests / introspection).
func (r *CommandRegistry) SlashCommands() []Command {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Command, 0, len(r.cmds))
	for _, c := range r.cmds {
		if c.Slash {
			out = append(out, c)
		}
	}
	return out
}

package watch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Sentinel errors for callers that need to tell one failure from another.
var (
	// ErrInvalid means the spec describes no runnable watch.
	ErrInvalid = errors.New("invalid watch")
	// ErrTooMany means MaxLive watches are already running.
	ErrTooMany = errors.New("too many watches")
	// ErrNotFound means no watch has that id.
	ErrNotFound = errors.New("no such watch")
	// ErrClosed means the manager is shut down.
	ErrClosed = errors.New("watch manager closed")
)

const (
	// MaxLive bounds how many watches run at once. Past it Start fails rather
	// than queueing: a watch nobody started is worse than an error saying so.
	MaxLive = 8
	// MinInterval floors a polling watch. Anything faster is a rate-limit
	// incident waiting to happen, and no remote state changes that fast.
	MinInterval = 5 * time.Second
	// logLimit is how many events one watch keeps for later reading.
	logLimit = 200
	// floodLimit and floodWindow stop a watch that became a firehose. A filter
	// matching every line is a bug in the filter, and its cost lands on the
	// model's context, so it is capped here rather than apologized for later.
	floodLimit  = 20
	floodWindow = time.Minute
	// eventTextLimit truncates one event: a watch delivers a signal, not a log.
	eventTextLimit = 2000
	// subscriberBuffer is how far one slow consumer may fall behind.
	subscriberBuffer = 64
)

// Trigger says what counts as an event from a streaming command.
type Trigger string

// Trigger values.
const (
	// OnLine emits every matching line while the command runs.
	OnLine Trigger = "line"
	// OnExit emits once, when the command finishes, with its tail.
	OnExit Trigger = "exit"
)

// Spec describes one watch. Command and Every decide its shape; see the
// package doc for the three combinations and what each is for.
type Spec struct {
	// Label says what is being watched, in the words of whoever started it.
	// It is what a timer emits and what every event is titled with.
	Label string
	// Command is the shell command to run. Empty makes a bare timer.
	Command string
	// Match filters lines by regexp. Empty matches every line.
	Match string
	// On decides what a streaming command emits. Empty means OnLine.
	On Trigger
	// Every turns a stream into a poll and a bare label into a timer.
	Every time.Duration
	// Once stops the watch after its first tick — a one-shot reminder.
	Once bool
}

// Event is one thing that happened.
type Event struct {
	ID    string
	Label string
	Text  string
	Time  time.Time
	// Final marks the last event under this ID: the watch exited, failed, was
	// stopped, or flooded. Nothing further arrives from it.
	Final bool
}

// Watch is a list snapshot.
type Watch struct {
	ID      string
	Label   string
	Command string
	Every   time.Duration
	On      Trigger
	Started time.Time
	Events  int
	Live    bool
	// Err is why a watch is no longer live, when it ended badly.
	Err string
}

// ShellResult is what a command run reports back.
type ShellResult struct {
	ExitCode int
	Canceled bool
}

// ShellFunc runs a shell command and streams its combined output. It is the
// seam a test replaces to run watches without a shell.
type ShellFunc func(ctx context.Context, command string, onChunk func(string)) (ShellResult, error)

// Options configures a Manager. The zero value is usable: it runs commands
// through the bash tool's shell, in the process working directory.
type Options struct {
	// Shell runs commands. Nil selects the bash tool's shell, so a watch runs
	// exactly what the bash tool would run.
	Shell ShellFunc
	// Cwd is where commands run, read at start time so a session that moves
	// takes the new directory with it. Nil means the process directory.
	Cwd func() string
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

// Manager owns every live watch and fans their events out to subscribers.
type Manager struct {
	shell ShellFunc
	cwd   func() string
	now   func() time.Time

	mu      sync.Mutex
	seq     int
	entries map[string]*entry
	order   []string
	subs    []*subscriber
	closed  bool

	wg sync.WaitGroup
}

type entry struct {
	Watch
	cancel context.CancelFunc
	log    []Event
	// windowStart and windowCount are the flood budget for this watch.
	windowStart time.Time
	windowCount int
	flooded     bool
}

type subscriber struct {
	ch     chan Event
	closed bool
}

// New returns a Manager with no watches running.
func New(opts Options) *Manager {
	m := &Manager{
		shell:   opts.Shell,
		cwd:     opts.Cwd,
		now:     opts.Now,
		entries: make(map[string]*entry),
	}
	if m.shell == nil {
		m.shell = defaultShell
	}
	if m.now == nil {
		m.now = time.Now
	}
	return m
}

// Start validates the spec, launches it, and returns the new watch.
func (m *Manager) Start(spec Spec) (Watch, error) {
	spec, match, err := normalize(spec)
	if err != nil {
		return Watch{}, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Watch{}, ErrClosed
	}
	if m.liveCountLocked() >= MaxLive {
		m.mu.Unlock()
		return Watch{}, fmt.Errorf("%w: %d already running — stop one first", ErrTooMany, MaxLive)
	}
	m.seq++
	id := fmt.Sprintf("w%d", m.seq)
	ctx, cancel := context.WithCancel(context.Background())
	e := &entry{
		Watch: Watch{
			ID:      id,
			Label:   spec.Label,
			Command: spec.Command,
			Every:   spec.Every,
			On:      spec.On,
			Started: m.now(),
			Live:    true,
		},
		cancel:      cancel,
		windowStart: m.now(),
	}
	m.entries[id] = e
	m.order = append(m.order, id)
	snapshot := e.Watch
	m.mu.Unlock()

	src := newSource(spec, match, m.shellFor(spec))
	m.wg.Add(1)
	go m.run(ctx, e, src)
	return snapshot, nil
}

// shellFor binds the manager's shell to the working directory a command runs
// in, so a source never has to know where that came from.
func (m *Manager) shellFor(Spec) ShellFunc {
	dir := ""
	if m.cwd != nil {
		dir = m.cwd()
	}
	return func(ctx context.Context, command string, onChunk func(string)) (ShellResult, error) {
		return m.shell(withCwd(ctx, dir), command, onChunk)
	}
}

func (m *Manager) run(ctx context.Context, e *entry, src Source) {
	defer m.wg.Done()
	err := src.Run(ctx, func(text string) { m.emit(e, text) })
	m.finish(e, err)
}

// emit records one event and fans it out, unless this watch has spent its
// flood budget — in which case it is stopped instead, and says so.
func (m *Manager) emit(e *entry, text string) {
	text = truncate(strings.TrimRight(text, "\n"), eventTextLimit)
	if strings.TrimSpace(text) == "" {
		return
	}

	m.mu.Lock()
	now := m.now()
	if now.Sub(e.windowStart) >= floodWindow {
		e.windowStart, e.windowCount = now, 0
	}
	e.windowCount++
	if e.windowCount > floodLimit {
		e.flooded = true
		m.mu.Unlock()
		e.cancel()
		return
	}
	ev := Event{ID: e.ID, Label: e.Label, Text: text, Time: now}
	e.Events++
	e.log = append(e.log, ev)
	if len(e.log) > logLimit {
		e.log = slices.Delete(e.log, 0, len(e.log)-logLimit)
	}
	m.publishLocked(ev)
	m.mu.Unlock()
}

// finish marks a watch done and publishes its last event, so a subscriber
// never has to poll to learn that nothing more is coming.
func (m *Manager) finish(e *entry, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.cancel()
	e.Live = false
	text := fmt.Sprintf("watch %s ended: %s", e.ID, e.Label)
	switch {
	case e.flooded:
		e.Err = fmt.Sprintf("more than %d events a minute", floodLimit)
		text = fmt.Sprintf("watch %s stopped itself: %s. It matched more than %d events a minute — "+
			"narrow the filter and start it again.", e.ID, e.Label, floodLimit)
	case err != nil && !errors.Is(err, context.Canceled):
		e.Err = err.Error()
		text = fmt.Sprintf("watch %s failed: %s (%s)", e.ID, e.Label, err)
	}
	ev := Event{ID: e.ID, Label: e.Label, Text: text, Time: m.now(), Final: true}
	e.log = append(e.log, ev)
	if len(e.log) > logLimit {
		e.log = slices.Delete(e.log, 0, len(e.log)-logLimit)
	}
	m.publishLocked(ev)
}

// Stop ends one watch. Stopping an already finished watch is not an error:
// the caller wanted it not running, and it is not running.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	e, ok := m.entries[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	e.cancel()
	return nil
}

// List returns every watch in start order, live or not.
func (m *Manager) List() []Watch {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Watch, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.entries[id].Watch)
	}
	return out
}

// Log returns the last events of one watch, oldest first.
func (m *Manager) Log(id string, limit int) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if limit <= 0 || limit > len(e.log) {
		limit = len(e.log)
	}
	return slices.Clone(e.log[len(e.log)-limit:]), nil
}

// Live reports how many watches are still running.
func (m *Manager) Live() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.liveCountLocked()
}

func (m *Manager) liveCountLocked() int {
	n := 0
	for _, e := range m.entries {
		if e.Live {
			n++
		}
	}
	return n
}

// Subscribe receives live events. The channel is buffered; a slow consumer
// misses events rather than stalling a watch. Cancel closes and unregisters it.
func (m *Manager) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	sub := &subscriber{ch: ch}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	m.subs = append(m.subs, sub)
	m.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			for i, s := range m.subs {
				if s == sub {
					m.subs = slices.Delete(m.subs, i, i+1)
					break
				}
			}
			if !sub.closed {
				sub.closed = true
				close(ch)
			}
		})
	}
}

// publishLocked fans one event out without blocking: a watch must not stall
// because something downstream stopped reading. The caller holds mu.
func (m *Manager) publishLocked(ev Event) {
	for _, sub := range m.subs {
		if sub.closed {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
		}
	}
}

// Close stops every watch and waits for their goroutines, then closes every
// subscriber channel. It is idempotent.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	for _, e := range m.entries {
		e.cancel()
	}
	m.mu.Unlock()

	m.wg.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sub := range m.subs {
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
	}
	m.subs = nil
}

// normalize validates a spec and fills its defaults, returning the compiled
// filter alongside so the source never parses anything.
func normalize(spec Spec) (Spec, *regexp.Regexp, error) {
	spec.Label = strings.TrimSpace(spec.Label)
	spec.Command = strings.TrimSpace(spec.Command)
	spec.Match = strings.TrimSpace(spec.Match)

	if spec.Command == "" && spec.Every <= 0 {
		return spec, nil, fmt.Errorf("%w: needs a command, an interval, or both", ErrInvalid)
	}
	if spec.Command == "" && spec.Label == "" {
		return spec, nil, fmt.Errorf("%w: a timer needs a label to come back with", ErrInvalid)
	}
	if spec.Every > 0 && spec.Every < MinInterval {
		return spec, nil, fmt.Errorf("%w: interval %s is below the %s floor", ErrInvalid, spec.Every, MinInterval)
	}
	if spec.Once && spec.Every <= 0 {
		return spec, nil, fmt.Errorf("%w: once needs an interval to fire after", ErrInvalid)
	}
	switch spec.On {
	case "", OnLine:
		spec.On = OnLine
	case OnExit:
		if spec.Every > 0 {
			return spec, nil, fmt.Errorf(
				"%w: on=exit is for a command that runs once, not every %s", ErrInvalid, spec.Every)
		}
	default:
		return spec, nil, fmt.Errorf("%w: unknown trigger %q", ErrInvalid, spec.On)
	}
	if spec.Label == "" {
		spec.Label = spec.Command
	}

	var match *regexp.Regexp
	if spec.Match != "" {
		var err error
		if match, err = regexp.Compile(spec.Match); err != nil {
			return spec, nil, fmt.Errorf("%w: match %q: %w", ErrInvalid, spec.Match, err)
		}
	}
	return spec, match, nil
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "\n… (truncated)"
}

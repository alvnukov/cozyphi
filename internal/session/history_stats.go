package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// HistoryTotals contains observed assistant-round usage, never price estimates.
// CachedTokens is a subset of InputTokens. TotalTokens is input plus output.
// UnknownUsage counts rounds with absent, zero, malformed or inconsistent usage;
// those rounds contribute no tokens. UnknownDates rounds cannot be period-filtered.
type HistoryTotals struct {
	InputTokens   int
	OutputTokens  int
	CachedTokens  int
	TotalTokens   int
	Rounds        int
	UnknownUsage  int
	UnknownModels int
	UnknownDates  int
}

// HistoryDay is a UTC calendar-day aggregate. Day "unknown" means no usable date.
type HistoryDay struct {
	Day string
	HistoryTotals
}

// HistoryModel aggregates persisted per-round model IDs; "unknown" is not inferred
// from the session's initial model because models can change during a session.
type HistoryModel struct {
	Model string
	HistoryTotals
}

// HistoryStatistics describes persisted history only, not unflushed live usage.
// Sessions counts distinct journal header IDs with activity in the period (a
// header creation or assistant round), including unknown-date activity. ActiveDays
// counts known UTC days containing assistant rounds. Days and Models are sorted
// lexically. Partial and bounded, sorted Warnings expose incomplete observations.
type HistoryStatistics struct {
	HistoryTotals
	Sessions   int
	ActiveDays int
	Days       []HistoryDay
	Models     []HistoryModel
	Partial    bool
	Warnings   []string
}

const (
	historyMaxFiles = 4096
	historyMaxLines = 200000
	historyMaxBytes = 256 * 1024 * 1024
)

// HistoryStats reads ~/.cozyphi/session/<encoded-cwd> without creating directories,
// repairing torn tails, loading message bodies or replaying transcript state.
// Cwd is made absolute and matched against journal headers (not git-root scoped).
// Since is inclusive; zero means all time. Unknown dates remain included, explicitly
// partial, because assigning them to either side of the cutoff would invent data.
//
// Shared nonempty assistant entry IDs are counted once across all matching journals
// in this scan, including branches and compacted/deleted context. The first record
// in lexical filename / physical line order wins; this is not cross-cwd dedup.
// Missing IDs cannot be deduplicated and are counted individually with a warning.
// Each scan is capped at 4096 directory entries, 200000 lines, 256 MiB total and
// 8 MiB per line. Oversized directories return partial rather than choosing an
// unstable subset. Concurrent appends are not a transactional snapshot.
func HistoryStats(ctx context.Context, cwd string, since time.Time) (HistoryStatistics, error) {
	if err := ctx.Err(); err != nil {
		return HistoryStatistics{}, err
	}
	if strings.TrimSpace(cwd) == "" {
		return HistoryStatistics{}, errors.New("session history: cwd must not be empty")
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return HistoryStatistics{}, fmt.Errorf("session history: resolve cwd: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return HistoryStatistics{}, fmt.Errorf("session history: resolve home: %w", err)
	}
	// Match project.ProjectDirName without importing project (which imports session).
	name := strings.TrimPrefix(strings.TrimPrefix(cwd, "/"), `\`)
	name = strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(name)
	if name == "" {
		name = "unknown"
	}
	dir := filepath.Join(home, ".cozyphi", "session", "--"+name+"--")
	return scanHistory(ctx, dir, cwd, since)
}

type historyScan struct {
	stats    HistoryStatistics
	days     map[string]HistoryTotals
	models   map[string]HistoryTotals
	seen     map[string]bool
	sessions map[string]bool
	warnings map[string]bool
	lines    int
	bytes    int64
}

func (s *historyScan) warn(message string) { s.warnings[message] = true }

func scanHistory(ctx context.Context, dir, cwd string, since time.Time) (HistoryStatistics, error) {
	s := historyScan{
		days: make(map[string]HistoryTotals), models: make(map[string]HistoryTotals),
		seen: make(map[string]bool), sessions: make(map[string]bool), warnings: make(map[string]bool),
	}
	f, err := os.Open(dir)
	if os.IsNotExist(err) {
		return s.stats, ctx.Err()
	}
	if err != nil {
		return s.stats, fmt.Errorf("session history: open directory: %w", err)
	}
	entries, readErr := f.ReadDir(historyMaxFiles + 1)
	closeErr := f.Close()
	if readErr != nil && readErr != io.EOF {
		return s.stats, fmt.Errorf("session history: list directory: %w", readErr)
	}
	if closeErr != nil {
		return s.stats, fmt.Errorf("session history: close directory: %w", closeErr)
	}
	if len(entries) > historyMaxFiles {
		s.warn("directory entry limit reached; history was not scanned")
		entries = nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return s.finish(), err
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") || entry.IsDir() {
			continue
		}
		if !entry.Type().IsRegular() {
			s.warn("non-regular journals skipped")
			continue
		}
		if s.lines >= historyMaxLines || s.bytes >= historyMaxBytes {
			s.warn("history scan limit reached")
			break
		}
		if err := s.file(ctx, filepath.Join(dir, entry.Name()), cwd, since); err != nil {
			return s.finish(), err
		}
	}
	return s.finish(), ctx.Err()
}

// Only metadata fields are decoded; content and tool payloads are never replayed.
type historyEntry struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Cwd       string          `json:"cwd"`
	Timestamp json.RawMessage `json:"timestamp"`
	Model     json.RawMessage `json:"model"`
	Usage     json.RawMessage `json:"usage"`
	Message   struct {
		Role llm.Role `json:"role"`
	} `json:"message"`
}

func (s *historyScan) file(ctx context.Context, path, cwd string, since time.Time) error {
	f, err := os.Open(path)
	if err != nil {
		s.warn("unreadable journals skipped")
		return nil
	}
	defer func() {
		if err := f.Close(); err != nil {
			s.warn("journal close failed")
		}
	}()
	remaining := int64(historyMaxBytes) - s.bytes
	r := &io.LimitedReader{R: f, N: remaining}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxEntryLine)
	header := false
	sessionID := ""
	for s.lines < historyMaxLines && sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.lines++
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e historyEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			s.warn("malformed journal records skipped (possibly a torn tail)")
			continue
		}
		if !header {
			if e.Type != EntrySession || e.Cwd == "" {
				s.warn("journals with missing or malformed headers skipped")
				break
			}
			header = true
			if filepath.Clean(e.Cwd) != cwd {
				break
			}
			sessionID = e.ID
			if sessionID == "" {
				sessionID = path
				s.warn("missing session IDs; counted by journal file")
			}
			date := historyDate(e.Timestamp, true)
			if date.IsZero() {
				s.warn("missing or invalid session dates")
			}
			if date.IsZero() || !date.Before(since) {
				s.sessions[sessionID] = true
			}
			continue
		}
		if e.Type != EntryMessage || e.Message.Role != llm.RoleAssistant {
			continue
		}
		duplicate := e.ID != "" && s.seen[e.ID]
		if e.ID != "" {
			s.seen[e.ID] = true
		}
		date := historyDate(e.Timestamp, false)
		if !date.IsZero() && date.Before(since) {
			continue
		}
		s.sessions[sessionID] = true
		if duplicate {
			continue
		}
		if e.ID == "" {
			s.warn("missing entry IDs; rounds cannot be deduplicated")
		}
		s.round(e, date)
	}
	s.bytes += remaining - r.N
	if !header {
		s.warn("journals without matching readable headers skipped")
	}
	if sc.Err() != nil {
		s.warn("journal read failed or line exceeded 8 MiB")
	}
	if r.N == 0 || s.lines >= historyMaxLines {
		s.warn("history scan limit reached")
	}
	return ctx.Err()
}

func historyDate(raw json.RawMessage, header bool) time.Time {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return time.Time{}
	}
	date, err := time.Parse(time.RFC3339Nano, text)
	if err != nil && header {
		// Historical headers omit a zone; UTC is the documented deterministic convention.
		date, _ = time.Parse("2006-01-02T15-04-05", text)
	}
	return date
}

func (s *historyScan) round(e historyEntry, date time.Time) {
	t := HistoryTotals{Rounds: 1}
	day := "unknown"
	if date.IsZero() {
		t.UnknownDates = 1
		s.warn("missing or invalid round dates; included in unknown day regardless of period")
	} else {
		day = date.UTC().Format("2006-01-02")
	}
	var model string
	if json.Unmarshal(e.Model, &model) != nil || strings.TrimSpace(model) == "" {
		model = "unknown"
		t.UnknownModels = 1
		s.warn("missing or invalid per-round models")
	}
	var u llm.Usage
	var required struct {
		Input  *int `json:"prompt_tokens"`
		Output *int `json:"completion_tokens"`
	}
	// Bound individual counters so even the maximum line count cannot overflow totals.
	maxTokens := int(^uint(0)>>1) / historyMaxLines / 2
	if json.Unmarshal(e.Usage, &required) != nil || required.Input == nil || required.Output == nil ||
		json.Unmarshal(e.Usage, &u) != nil || u.PromptTokens < 0 || u.CompletionTokens < 0 ||
		u.PromptTokens > maxTokens || u.CompletionTokens > maxTokens ||
		u.CachedTokens() < 0 || u.CachedTokens() > u.PromptTokens || u.TotalTokens < 0 ||
		(u.PromptTokens == 0 && u.CompletionTokens == 0) ||
		(u.TotalTokens != 0 && u.TotalTokens != u.PromptTokens+u.CompletionTokens) {
		t.UnknownUsage = 1
		s.warn("missing, zero, malformed or inconsistent assistant usage; token totals are partial")
	} else {
		t.InputTokens = u.PromptTokens
		t.OutputTokens = u.CompletionTokens
		t.CachedTokens = u.CachedTokens()
		t.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	s.stats.HistoryTotals = addHistoryTotals(s.stats.HistoryTotals, t)
	s.days[day] = addHistoryTotals(s.days[day], t)
	s.models[model] = addHistoryTotals(s.models[model], t)
}

func addHistoryTotals(a, b HistoryTotals) HistoryTotals {
	a.InputTokens += b.InputTokens
	a.OutputTokens += b.OutputTokens
	a.CachedTokens += b.CachedTokens
	a.TotalTokens += b.TotalTokens
	a.Rounds += b.Rounds
	a.UnknownUsage += b.UnknownUsage
	a.UnknownModels += b.UnknownModels
	a.UnknownDates += b.UnknownDates
	return a
}

func (s *historyScan) finish() HistoryStatistics {
	s.stats.Sessions = len(s.sessions)
	for day, totals := range s.days {
		s.stats.Days = append(s.stats.Days, HistoryDay{Day: day, HistoryTotals: totals})
		if day != "unknown" {
			s.stats.ActiveDays++
		}
	}
	for model, totals := range s.models {
		s.stats.Models = append(s.stats.Models, HistoryModel{Model: model, HistoryTotals: totals})
	}
	for warning := range s.warnings {
		s.stats.Warnings = append(s.stats.Warnings, warning)
	}
	sort.Slice(s.stats.Days, func(i, j int) bool { return s.stats.Days[i].Day < s.stats.Days[j].Day })
	sort.Slice(s.stats.Models, func(i, j int) bool { return s.stats.Models[i].Model < s.stats.Models[j].Model })
	sort.Strings(s.stats.Warnings)
	s.stats.Partial = len(s.stats.Warnings) > 0
	return s.stats
}

package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// EntrySessionTitle is metadata, not a node in the conversational chain.
const EntrySessionTitle = "session_title"

// SessionTitleEntry records the latest explicit name and its owner.
type SessionTitleEntry struct {
	SessionBaseEntry
	Title  string `json:"title"`
	Source string `json:"source"`
}

func (SessionTitleEntry) GetType() string    { return EntrySessionTitle }
func (e SessionTitleEntry) GetID() string    { return e.ID }
func (SessionTitleEntry) GetParent() *string { return nil }

// ErrTitlePinned prevents automated renaming from undoing a user's choice.
var ErrTitlePinned = errors.New("title is pinned by the user; ask the user to run /rename")

func normalizeTitle(title, source string) (string, error) {
	if source != "user" && source != "model" {
		return "", errors.New("session: title source must be user or model")
	}
	if !utf8.ValidString(title) {
		return "", errors.New("session: title must be valid UTF-8")
	}
	for _, r := range title {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return "", errors.New("session: title must be a single line without control or formatting characters")
		}
	}
	title = strings.Join(strings.Fields(title), " ")
	if title == "" || utf8.RuneCountInString(title) > 60 {
		return "", errors.New("session: title must contain 1 to 60 characters")
	}
	return title, nil
}

// SetTitle persists metadata immediately, without moving the conversational leaf.
// The ownership check and append share the lock so a late model cannot undo /rename.
func (sm *Manager) SetTitle(title, source string) error {
	title, err := normalizeTitle(title, source)
	if err != nil {
		return err
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.closed {
		return os.ErrClosed
	}
	if source == "model" && sm.titleSource == "user" {
		return ErrTitlePinned
	}
	if sm.title == title && sm.titleSource == source {
		return nil
	}
	entry := SessionTitleEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntrySessionTitle, ID: sm.generateID(), Timestamp: time.Now()},
		Title:            title, Source: source,
	}
	previousLen := len(sm.entries)
	sm.entries = append(sm.entries, entry)
	sm.byIDs[entry.ID] = entry
	if sm.config.shouldFlush {
		if err := sm.flush(entry); err != nil {
			sm.entries = sm.entries[:previousLen]
			delete(sm.byIDs, entry.ID)
			return fmt.Errorf("session: persist title: %w", err)
		}
	}
	sm.title, sm.titleSource = title, source
	return nil
}

// Title returns the explicit title and owner; empty means no explicit title.
func (sm *Manager) Title() (title, source string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.title, sm.titleSource
}

// DisplayTitle returns the shared display name, including legacy-session fallback.
func (sm *Manager) DisplayTitle() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	meta := SessionMeta{ID: sm.sessionID, Title: sm.title}
	if meta.Title == "" {
		for _, entry := range sm.entries {
			if msg, ok := entry.(SessionMessageEntry); ok {
				meta.FirstPrompt = titlePrompt(msg)
				if meta.FirstPrompt != "" {
					break
				}
			}
		}
	}
	return DisplayTitle(meta)
}

// DisplayTitle uses the explicit name, then first prompt, then short ID.
// All paths are bounded and safe for plain terminal display, including old logs.
func DisplayTitle(meta SessionMeta) string {
	if title := displayText(meta.Title, 60); title != "" {
		return title
	}
	if prompt := displayText(meta.FirstPrompt, 48); prompt != "" {
		return prompt
	}
	return displayText(meta.ID, 8)
}

func titlePrompt(entry SessionMessageEntry) string {
	if entry.Message.Role != llm.RoleUser || entry.DeliveryID != "" {
		return ""
	}
	text := strings.TrimSpace(entry.Message.Content)
	// Harness deliveries are not the user's goal. Older logs lack delivery IDs.
	for strings.HasPrefix(text, "<system-reminder>") {
		_, after, found := strings.Cut(text, "</system-reminder>")
		if !found {
			return ""
		}
		text = strings.TrimSpace(after)
	}
	return displayText(text, 48)
}

func displayText(text string, limit int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, text)
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

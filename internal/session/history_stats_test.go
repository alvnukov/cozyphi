package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

func historyFixture(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "project")
	dir := project.ProjectSessionDir(filepath.Join(home, ".cozyphi", "session"), cwd)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return cwd, dir
}

func historyHeader(id, cwd string) string {
	b, _ := json.Marshal(session.SessionHeader{
		Type: session.EntrySession, ID: id, Cwd: cwd, Timestamp: "2025-01-01T00-00-00", Model: "not-a-fallback",
	})
	return string(b) + "\n"
}

func historyRound(id, date, model, usage string) string {
	return `{"type":"EntryMessage","id":"` + id + `","timestamp":"` + date +
		`","model":"` + model + `","message":{"role":"assistant","content":"not replayed"},"usage":` + usage + "}\n"
}

func historyWrite(t *testing.T, dir, name, data string) string {
	t.Helper()
	path := filepath.Join(dir, name+".jsonl")
	if err := os.WriteFile(path, []byte(data), 0o400); err != nil {
		t.Fatal(err)
	}
	return path
}

const historyUsage = `{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,` +
	`"prompt_tokens_details":{"cached_tokens":60}}`

func TestHistoryStatsPeriodModelsAndForkDedup(t *testing.T) {
	cwd, dir := historyFixture(t)
	shared := historyRound("shared", "2025-02-02T01:00:00+02:00", "z-model", historyUsage)
	historyWrite(t, dir, "a", historyHeader("original", cwd)+shared+
		historyRound("old", "2025-01-01T00:00:00Z", "old-model", historyUsage)+
		`{"type":"EntryMessage","id":"user","timestamp":"2025-02-01T00:00:00Z",`+
		`"message":{"role":"user"},"usage":`+historyUsage+"}\n")
	historyWrite(t, dir, "b", historyHeader("fork", cwd)+shared+
		historyRound("new", "2025-02-02T12:00:00Z", "a-model", historyUsage))
	historyWrite(t, dir, "c", historyHeader("other", cwd+"-other")+
		historyRound("foreign", "2025-02-03T12:00:00Z", "other", historyUsage))
	since := time.Date(2025, 2, 1, 23, 0, 0, 0, time.UTC)
	got, err := session.HistoryStats(t.Context(), cwd, since)
	if err != nil {
		t.Fatal(err)
	}
	if got.Rounds != 2 || got.Sessions != 2 || got.ActiveDays != 2 || got.InputTokens != 200 ||
		got.OutputTokens != 40 || got.CachedTokens != 120 || got.TotalTokens != 240 {
		t.Fatalf("unexpected totals: %+v", got)
	}
	if len(got.Days) != 2 || got.Days[0].Day != "2025-02-01" || got.Days[1].Day != "2025-02-02" ||
		len(got.Models) != 2 || got.Models[0].Model != "a-model" || got.Models[1].Model != "z-model" {
		t.Fatalf("unexpected groups: %+v", got)
	}
	if got.Partial {
		t.Fatalf("complete metadata marked partial: %v", got.Warnings)
	}
	again, err := session.HistoryStats(t.Context(), cwd, since)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("non-deterministic result: %+v, %v", again, err)
	}
	all, err := session.HistoryStats(t.Context(), cwd, time.Time{})
	if err != nil || all.Rounds != 3 || all.TotalTokens != 360 {
		t.Fatalf("all time: %+v, %v", all, err)
	}
}

func TestHistoryStatsPartialAndReadOnly(t *testing.T) {
	cwd, dir := historyFixture(t)
	data := historyHeader("partial", cwd) +
		historyRound("unknown", "bad-date", "", `null`) +
		historyRound("zero", "2025-02-01T00:00:00Z", "model", `{}`) +
		historyRound("bad-cache", "2025-02-01T00:00:00Z", "model",
			`{"prompt_tokens":1,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":9}}`) +
		historyRound("bad-usage", "2025-02-01T00:00:00Z", "model", `"oops"`) +
		historyRound("", "2025-02-01T00:00:00Z", "model", historyUsage) +
		`{"type":"EntryMessage","id":"old-format","message":{"role":"assistant"}}` + "\n" +
		`{"type":"EntryMessage"`
	path := historyWrite(t, dir, "partial", data)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := session.HistoryStats(t.Context(), cwd, time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Partial || got.UnknownDates != 2 || got.UnknownModels != 2 || got.UnknownUsage != 5 ||
		got.Rounds != 6 || got.TotalTokens != 120 || got.Days[len(got.Days)-1].Day != "unknown" {
		t.Fatalf("partial metadata: %+v", got)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != data || !before.ModTime().Equal(after.ModTime()) ||
		before.Mode() != after.Mode() {
		t.Fatalf("journal mutated: %v", err)
	}
}

func TestHistoryStatsMissingAndCancellation(t *testing.T) {
	cwd, _ := historyFixture(t)
	got, err := session.HistoryStats(t.Context(), cwd+"-missing", time.Time{})
	if err != nil || got.Partial || got.Sessions != 0 {
		t.Fatalf("missing history: %+v, %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = session.HistoryStats(ctx, cwd, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := session.HistoryStats(t.Context(), "", time.Time{}); err == nil {
		t.Fatal("empty cwd accepted")
	}
}

func TestHistoryStatsBoundedLine(t *testing.T) {
	cwd, dir := historyFixture(t)
	historyWrite(t, dir, "large", historyHeader("large", cwd)+strings.Repeat("x", 8*1024*1024+1))
	got, err := session.HistoryStats(t.Context(), cwd, time.Time{})
	if err != nil || !got.Partial || len(got.Warnings) == 0 {
		t.Fatalf("oversized line: %+v, %v", got, err)
	}
}

func TestHistoryStatsUsageValidation(t *testing.T) {
	cwd, dir := historyFixture(t)
	usages := []string{
		`{"prompt_tokens":1}`,
		`{"prompt_tokens":null,"completion_tokens":1}`,
		`{"prompt_tokens":-1,"completion_tokens":1}`,
		`{"prompt_tokens":1,"completion_tokens":1,"total_tokens":9}`,
		`{"prompt_tokens":9223372036854775807,"completion_tokens":1}`,
		`{"prompt_tokens":3,"completion_tokens":2}`,
	}
	data := historyHeader("usage", cwd)
	for i, usage := range usages {
		data += historyRound(string(rune('a'+i)), "2025-02-01T00:00:00Z", "model", usage)
	}
	historyWrite(t, dir, "usage", data)
	got, err := session.HistoryStats(t.Context(), cwd, time.Time{})
	if err != nil || got.UnknownUsage != 5 || got.TotalTokens != 5 || got.Rounds != 6 {
		t.Fatalf("usage validation: %+v, %v", got, err)
	}
}

// Cancellation is triggered at a deterministic check inside the scan, without sleeps.
type historyCancelContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *historyCancelContext) Err() error {
	c.checks++
	if c.checks == 6 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestHistoryStatsCancellationDuringScan(t *testing.T) {
	cwd, dir := historyFixture(t)
	data := historyHeader("cancel", cwd)
	for i := range 20 {
		data += historyRound(string(rune('a'+i)), "2025-02-01T00:00:00Z", "model", historyUsage)
	}
	historyWrite(t, dir, "cancel", data)
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &historyCancelContext{Context: base, cancel: cancel}
	got, err := session.HistoryStats(ctx, cwd, time.Time{})
	if !errors.Is(err, context.Canceled) || got.Rounds == 0 || got.Rounds >= 20 {
		t.Fatalf("mid-scan cancellation: %+v, %v", got, err)
	}
}

func TestHistoryStatsDedupFirstRecordWins(t *testing.T) {
	cwd, dir := historyFixture(t)
	historyWrite(t, dir, "b", historyHeader("b", cwd)+
		historyRound("shared", "2025-02-01T00:00:00Z", "second", historyUsage))
	historyWrite(t, dir, "a", historyHeader("a", cwd)+
		historyRound("shared", "2025-01-01T00:00:00Z", "first", historyUsage))
	since := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	got, err := session.HistoryStats(t.Context(), cwd, since)
	if err != nil || got.Rounds != 0 {
		t.Fatalf("first record should win before period filtering: %+v, %v", got, err)
	}
}

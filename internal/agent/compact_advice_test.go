package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
)

func TestCompactAdviceReminderCarriesNumbersAndChecklist(t *testing.T) {
	got := compactAdviceReminder(compactAdviceFromPressure, 800000, 1000000)
	require.True(t, strings.HasPrefix(got, reminderOpen), "must use the reminder wire format")
	require.True(t, strings.HasSuffix(got, reminderClose))
	require.Contains(t, got, "~800000 of 1000000")
	require.Contains(t, got, "must survive compaction")
	require.Contains(t, got, `context tool with {"action":"compact"}`)

	bare := compactAdviceReminder(compactAdviceFromPlan, 0, 0)
	require.NotContains(t, bare, "Context pressure:", "no numbers, no pressure line")
	require.Contains(t, bare, compactAdviceFromPlan)
}

// seedTwoTurnHistory reports 25000 provider tokens; a 30000-token window
// puts the default reminder threshold at 13616, so pressure is real.
func TestNoteCompactPressureAdvisesOncePerCrossing(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 30000)
	seedTwoTurnHistory(t, engine)

	engine.noteCompactPressure()
	require.NotEmpty(t, engine.compactAdvice, "pressure above the threshold queues the advice")
	require.Contains(t, engine.compactAdvice, compactAdviceFromPressure)
	first := engine.compactAdvice

	engine.noteCompactPressure()
	require.Equal(t, first, engine.compactAdvice, "the same crossing must not re-render the advice")

	// A compaction lands: the latch re-arms, the prompt drains the reminder,
	// and the next crossing — usage is still high — advises again.
	engine.rearmCompactAdvice()
	engine.drainCompactAdvice()
	engine.noteCompactPressure()
	require.NotEmpty(t, engine.compactAdvice, "a new crossing after a compaction advises again")
}

func TestNoteCompactPressureQuietBelowThreshold(t *testing.T) {
	// 100000-window: default threshold 83616 dwarfs the 25000-token seed.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	seedTwoTurnHistory(t, engine)
	engine.noteCompactPressure()
	require.Empty(t, engine.compactAdvice, "no advice below the threshold")
	require.False(t, engine.compactAdvised)

	// Estimate path too: no provider usage at all, bytes/4 stays tiny.
	bare := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, bare.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "hi"},
		llm.Message{Role: llm.RoleAssistant, Content: "ok"},
	))
	bare.noteCompactPressure()
	require.Empty(t, bare.compactAdvice)
}

func TestSetCompactionSettingsMovesReminderThreshold(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	seedTwoTurnHistory(t, engine) // 25000 tokens

	engine.noteCompactPressure()
	require.Empty(t, engine.compactAdvice, "default threshold stays quiet at 25000 of 100000")

	engine.SetCompactionSettings(compaction.ConfiguredSettings(20000))
	engine.noteCompactPressure()
	require.NotEmpty(t, engine.compactAdvice, "a lower user-set threshold advises early")
	require.Contains(t, engine.compactAdvice, "~25000")
}

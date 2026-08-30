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
func TestNoteCompactPressureEscalatesEveryTurn(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 30000)
	seedTwoTurnHistory(t, engine)

	engine.noteCompactPressure()
	require.NotEmpty(t, engine.compactAdvice, "pressure above the threshold queues the advice")
	require.Contains(t, engine.compactAdvice, compactAdviceFromPressure)
	require.False(t, engine.compactHardMode(), "the first reminders stay soft")
	soft := engine.compactAdvice
	require.NotEmpty(t, soft)

	// The old latch stayed silent until a compaction landed; the ladder
	// re-queues the reminder on every uncompacted turn instead.
	engine.drainCompactAdvice()
	engine.noteCompactPressure()
	require.NotEmpty(t, engine.compactAdvice, "every uncompacted turn re-queues the reminder")

	engine.noteCompactPressure() // strike 3: the executor starts blocking tools
	require.True(t, engine.compactHardMode())
	require.Contains(t, engine.compactAdvice, "blocked", "the hard directive names the block")
	require.Contains(t, engine.compactAdvice, "reminder 3")
	require.False(t, engine.compactStopActive(), "the model still gets the hard turn")

	engine.noteCompactPressure() // strike 4: the model ignored the hard mode
	require.True(t, engine.compactStopActive())

	// A compaction lands: the ladder resets from the bottom.
	engine.rearmCompactAdvice()
	require.False(t, engine.compactHardMode())
	require.False(t, engine.compactStopActive())
	require.Zero(t, engine.compactStrikes)
}

func TestNoteCompactPressureSupersedesPlanAdvice(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 30000)
	seedTwoTurnHistory(t, engine)

	// A plan compact action parked its nudge earlier in the turn.
	engine.queueCompactAdvice(compactAdviceFromPlan, 0, 0)
	require.Contains(t, engine.compactAdvice, compactAdviceFromPlan)

	// Turn-end pressure is the fresher fact: it replaces the parked advice
	// instead of being masked by it.
	engine.noteCompactPressure()
	require.Contains(t, engine.compactAdvice, compactAdviceFromPressure)
	require.Contains(t, engine.compactAdvice, "~25000")
}

func TestNoteCompactPressureQuietBelowThreshold(t *testing.T) {
	// 100000-window: default threshold 83616 dwarfs the 25000-token seed.
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	seedTwoTurnHistory(t, engine)
	engine.noteCompactPressure()
	require.Empty(t, engine.compactAdvice, "no advice below the threshold")
	require.Zero(t, engine.compactStrikes)

	// Estimate path too: no provider usage at all, bytes/4 stays tiny.
	bare := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	require.NoError(t, bare.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "hi"},
		llm.Message{Role: llm.RoleAssistant, Content: "ok"},
	))
	bare.noteCompactPressure()
	require.Empty(t, bare.compactAdvice)
	require.Zero(t, bare.compactStrikes)
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

func TestCompactGateForBlocksAllButContext(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 30000)
	seedTwoTurnHistory(t, engine)

	require.Empty(t, engine.compactGateFor("bash"), "no directive before the hard strike")
	require.Empty(t, engine.compactGateFor("context"))

	for range compactStrikesHard {
		engine.noteCompactPressure()
	}
	require.NotEmpty(t, engine.compactGateFor("bash"), "hard mode refuses working tools")
	require.Contains(t, engine.compactGateFor("bash"), `{"action":"compact"}`)
	require.Empty(t, engine.compactGateFor("context"), "the context tool stays runnable")

	engine.rearmCompactAdvice()
	require.Empty(t, engine.compactGateFor("bash"), "a compaction releases the gate")
}

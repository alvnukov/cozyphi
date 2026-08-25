package transcript

import (
	"fmt"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestTranscriptPaneStreamingUpdateWorkDoesNotScaleWithHistory(t *testing.T) {
	small := streamingUpdateAllocs(10)
	large := streamingUpdateAllocs(5_000)
	if large > small+50 {
		t.Fatalf("stream update allocations scale with history: 10 messages=%.0f, 5000 messages=%.0f", small, large)
	}
}

func BenchmarkTranscriptPaneStreamingUpdate(b *testing.B) {
	for _, history := range []int{10, 1_000, 5_000} {
		b.Run(fmt.Sprintf("history_%d", history), func(b *testing.B) {
			pane := benchmarkPane(history)
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				pane.ApplySession(streamingUpdate(i))
				pane.Sync()
			}
		})
	}
}

func streamingUpdateAllocs(history int) float64 {
	pane := benchmarkPane(history)
	i := 0
	return testing.AllocsPerRun(10, func() {
		pane.ApplySession(streamingUpdate(i))
		pane.Sync()
		i++
	})
}

func benchmarkPane(history int) *TranscriptPane {
	messages := make([]session.Message, 0, history+1)
	for i := range history {
		if i%2 == 0 {
			messages = append(messages, session.Message{
				ID:   fmt.Sprintf("user-%d", i),
				Role: session.RoleUser,
				Text: "historical prompt",
			})
			continue
		}
		messages = append(messages, session.Message{
			ID:    fmt.Sprintf("assistant-%d", i),
			Role:  session.RoleAssistant,
			State: session.StateComplete,
			Content: []session.ContentBlock{
				{Type: session.BlockText, Text: "historical answer"},
			},
		})
	}
	messages = append(messages, streamingUpdate(0).Message)
	pane := NewTranscriptPane(components.DefaultTheme(), nil, "test")
	pane.LoadReplay(session.Snapshot{Messages: messages})
	pane.Sync()
	return pane
}

func streamingUpdate(i int) session.AssistantMessageUpdate {
	return session.AssistantMessageUpdate{Message: session.Message{
		ID:    "assistant-current",
		State: session.StateStreaming,
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: fmt.Sprintf("stream token %d", i%2)},
		},
	}}
}

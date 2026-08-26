package questiontool_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
)

func TestToolAsksAndFormatsAnswers(t *testing.T) {
	var got []questiontool.Question
	tool := questiontool.Tool(questiontool.Deps{
		Ask: func(_ context.Context, questions []questiontool.Question) ([]questiontool.Answer, error) {
			got = questions
			return []questiontool.Answer{{"Build"}, {"A", "B"}}, nil
		},
	})
	require.Equal(t, "question", tool.Definition.Name)

	result, err := tool.Run(t.Context(), json.RawMessage(`{"questions":[
		{"question":"What mode?","header":"Mode","options":[{"label":"Build","description":"Do work"},{"label":"Plan","description":"Only plan"}]},
		{"question":"Pick","header":"Pick","options":[{"label":"A","description":"a"},{"label":"B","description":"b"}],"multiple":true}
	]}`))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Build", got[0].Options[0].Label)
	assert.True(t, got[1].Multiple)
	assert.Contains(t, result.Content, `"What mode?"="Build"`)
	assert.Contains(t, result.Content, `"Pick"="A, B"`)
	assert.Equal(t, "asked 2 question(s)", result.Detail)
	assert.Equal(
		t,
		"asked 1 question(s)",
		tool.DetailFromArgs(
			json.RawMessage(
				`{"questions":[{"question":"q","header":"h","options":[{"label":"a","description":"d"}]}]}`,
			),
		),
	)
}

func TestToolRequiresQuestions(t *testing.T) {
	calls := 0
	tool := questiontool.Tool(questiontool.Deps{
		Ask: func(context.Context, []questiontool.Question) ([]questiontool.Answer, error) {
			calls++
			return nil, nil
		},
	})
	_, err := tool.Run(t.Context(), json.RawMessage(`{"questions":[]}`))
	require.Error(t, err)
	_, err = tool.Run(t.Context(), json.RawMessage(`{}`))
	require.Error(t, err)
	_, err = tool.Run(
		t.Context(),
		json.RawMessage(
			`{"questions":[{"question":"q","header":"h","options":[{"label":"a","description":"d"}],"extra":true}]}`,
		),
	)
	require.Error(t, err) // unknown parameter
	assert.Zero(t, calls)
}

func TestToolDefaultsCustomTrueAndUnanswered(t *testing.T) {
	tool := questiontool.Tool(questiontool.Deps{
		Ask: func(_ context.Context, questions []questiontool.Question) ([]questiontool.Answer, error) {
			assert.True(t, questions[0].Custom)
			return []questiontool.Answer{nil}, nil
		},
	})
	result, err := tool.Run(
		t.Context(),
		json.RawMessage(`{"questions":[{"question":"q","header":"h","options":[{"label":"a","description":"d"}]}]}`),
	)
	require.NoError(t, err)
	assert.Contains(t, result.Content, "Unanswered")
}

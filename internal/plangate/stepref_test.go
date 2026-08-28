package plangate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestStepFromArgsReadsIdAndLegacyNumber(t *testing.T) {
	cases := []struct {
		name string
		args string
		want StepRef
	}{
		{name: "stable id", args: `{"plan_step":"wire-schema"}`, want: StepRef{ID: "wire-schema"}},
		{name: "legacy number", args: `{"plan_step":2}`, want: StepRef{Ordinal: 2}},
		{name: "absent", args: `{"path":"a.go"}`, want: StepRef{}},
		{name: "null", args: `{"plan_step":null}`, want: StepRef{}},
		{name: "empty object", args: `{}`, want: StepRef{}},
		{name: "wrong value type", args: `{"plan_step":true}`, want: StepRef{}},
		{name: "broken json", args: `{`, want: StepRef{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, StepFromArgs([]byte(tc.args)))
		})
	}
}

func TestStepRefFindResolvesIdThenOrdinal(t *testing.T) {
	plan := session.Plan{Items: []session.PlanItem{{ID: "alpha"}, {ID: "beta"}}}

	item, ok := StepRef{ID: "beta"}.Find(plan)
	require.True(t, ok)
	assert.Equal(t, "beta", item.ID)

	_, ok = StepRef{ID: "ghost"}.Find(plan)
	assert.False(t, ok, "an unknown id resolves to nothing")

	item, ok = StepRef{Ordinal: 1}.Find(plan)
	require.True(t, ok)
	assert.Equal(t, "alpha", item.ID)

	_, ok = StepRef{Ordinal: len(plan.Items) + 1}.Find(plan)
	assert.False(t, ok, "an out-of-range ordinal resolves to nothing")

	_, ok = StepRef{}.Find(plan)
	assert.False(t, ok, "an omitted reference resolves to nothing")
}

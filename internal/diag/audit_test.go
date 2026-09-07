package diag_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// recorder collects what a sink was handed, so a test can assert on the
// events themselves rather than on a rendering of them.
type recorder struct {
	events []diag.AuditEvent
}

func (r *recorder) sink(event diag.AuditEvent) { r.events = append(r.events, event) }

func (r *recorder) only(t *testing.T) diag.AuditEvent {
	t.Helper()
	require.Len(t, r.events, 1, "one request leaves exactly one record")
	return r.events[0]
}

// rendered is every line the sink was handed, which is what a reader of the
// debug log would actually see.
func (r *recorder) rendered() string {
	var b strings.Builder
	for _, event := range r.events {
		b.WriteString(event.Line())
		b.WriteString("\n")
	}
	return b.String()
}

func audited(collectors ...diag.Collector) (*diag.Registry, *recorder) {
	log := &recorder{}
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), collectors...).WithAudit(log.sink)
	return registry, log
}

// Every entry point leaves a record, including the ones that answer without
// observing anything: a reader asking what the model looked at needs the
// catalog calls too.
func TestEachEntryPointLeavesOneRecordOfWhatWasAsked(t *testing.T) {
	runtime := func() *fake {
		return &fake{
			category: diag.CategoryRuntime,
			status:   available("version"),
			fields:   []diag.Field{stringField("version", "1.2.3")},
		}
	}

	registry, log := audited(runtime())
	registry.Catalog()
	catalog := log.only(t)
	assert.Equal(t, diag.AuditCatalog, catalog.Action)
	assert.Equal(t, diag.AuditAnswered, catalog.Result)
	assert.Equal(t, len(diag.Categories()), catalog.Categories, "every category is listed")
	assert.Equal(t, 0, catalog.Fields, "and none of them is observed")

	registry, log = audited(runtime())
	_, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	detail := log.only(t)
	assert.Equal(t, diag.AuditSnapshot, detail.Action)
	assert.Equal(t, diag.CategoryRuntime, detail.Category)
	assert.Equal(t, string(diag.ModeDetail), detail.Mode)
	assert.Equal(t, diag.AuditAnswered, detail.Result)
	assert.Equal(t, 1, detail.Categories)
	assert.Equal(t, 1, detail.Fields)
	assert.False(t, detail.Partial)
	assert.False(t, detail.Truncated)

	registry, log = audited(runtime())
	_, err = registry.Snapshot(t.Context(), "")
	require.NoError(t, err)
	overview := log.only(t)
	assert.Equal(t, string(diag.ModeOverview), overview.Mode)
	assert.Empty(t, overview.Category, "the overview names no category")
	assert.Equal(t, len(diag.Categories()), overview.Categories)
	assert.False(t, overview.Partial,
		"a category with no collector is a documented gap, not an owner that could not be read")

	registry, log = audited(runtime())
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "version")
	require.NoError(t, err)
	explain := log.only(t)
	assert.Equal(t, diag.AuditExplain, explain.Action)
	assert.Equal(t, diag.CategoryRuntime, explain.Category)
	assert.Equal(t, "version", explain.Key)
	assert.Equal(t, diag.AuditAnswered, explain.Result)
	assert.Equal(t, 1, explain.Fields)
}

// A request turned down before anything was observed is refused, not failed:
// nothing was read, so nothing could have leaked, and a reader chasing a
// real fault should not have to sift these out of the failures.
func TestARequestNobodyCanAnswerIsRefusedRatherThanFailed(t *testing.T) {
	runtime := &fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		fields:   []diag.Field{stringField("version", "1.2.3")},
	}

	registry, log := audited(runtime)
	_, err := registry.Snapshot(t.Context(), "no-such-category")
	require.Error(t, err)
	assert.Equal(t, diag.AuditRefused, log.only(t).Result)

	registry, log = audited(runtime)
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "no-such-key")
	require.Error(t, err)
	unknownKey := log.only(t)
	assert.Equal(t, diag.AuditRefused, unknownKey.Result)
	assert.Equal(t, "no-such-key", unknownKey.Key, "what was asked for is worth recording")
	assert.Equal(t, 0, unknownKey.Fields)

	registry, log = audited(runtime)
	_, err = registry.Explain(t.Context(), diag.CategoryModel, "anything")
	require.Error(t, err)
	assert.Equal(t, diag.AuditRefused, log.only(t).Result, "a category with no collector observes nothing")

	registry, log = audited(&fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		err:      errors.New("owner is holding a lock"),
	})
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "version")
	require.Error(t, err)
	assert.Equal(t, diag.AuditFailed, log.only(t).Result, "an owner that could not be read did fail")
}

// The key is the one member of the record the model supplies, so it goes
// through the same sanitizing and bounding as any value before it is
// written anywhere.
func TestTheOnlyMemberTheModelSuppliesIsSanitized(t *testing.T) {
	runtime := &fake{category: diag.CategoryRuntime, status: available("version")}
	registry, log := audited(runtime)

	credential := "ghp_" + strings.Repeat("A", 36)
	_, err := registry.Explain(t.Context(), diag.CategoryRuntime, credential)
	require.Error(t, err)
	event := log.only(t)
	assert.NotContains(t, event.Key, credential, "a key shaped like a credential is masked")
	assert.Contains(t, event.Key, "[REDACTED]")
	assert.NotContains(t, event.Line(), credential)

	registry, log = audited(runtime)
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, strings.Repeat("k", 4096))
	require.Error(t, err)
	assert.LessOrEqual(t, len(log.only(t).Key), diag.DefaultMaxValueBytes,
		"a key long enough to be a payload is cut like one")
}

// The record says what was asked and how far it reached. What came back is
// the answer's business: no value, source ref, reason or error message has
// anywhere in the event to land.
func TestTheRecordCarriesNoPartOfTheAnswer(t *testing.T) {
	const secret = "sentinel-nothing-may-carry-this"
	leaky := &fake{
		category: diag.CategoryRuntime,
		status: diag.Status{
			Availability: diag.AvailabilityAvailable,
			Keys:         []string{"version"},
			Reason:       "reason mentioning " + secret,
		},
		fields: []diag.Field{stringField("version", secret)},
	}

	registry, log := audited(leaky)
	_, err := registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err)
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "version")
	require.NoError(t, err)
	registry.Catalog()

	require.Len(t, log.events, 3)
	assert.NotContains(t, log.rendered(), secret, "the value the owner held stays with the owner")

	broken := &fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		err:      errors.New("dial tcp 10.0.0.4:443: token " + secret),
	}
	registry, log = audited(broken)
	_, err = registry.Snapshot(t.Context(), diag.CategoryRuntime)
	require.NoError(t, err, "an owner that cannot answer is a hole in the answer, not an error")
	_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "version")
	require.Error(t, err)

	assert.NotContains(t, log.rendered(), secret, "an owner's error text is not recorded at all")
	assert.NotContains(t, log.rendered(), "10.0.0.4")
	assert.True(t, log.events[0].Partial, "how far the request reached is recorded instead")
	assert.Equal(t, diag.AuditFailed, log.events[1].Result)
}

// The sink is optional. A harness that wires none is not a harness with a
// broken audit, and every entry point has to survive that.
func TestARegistryWithNoSinkAnswersAnyway(t *testing.T) {
	runtime := &fake{
		category: diag.CategoryRuntime,
		status:   available("version"),
		fields:   []diag.Field{stringField("version", "1.2.3")},
	}
	for name, registry := range map[string]*diag.Registry{
		"never wired": diag.NewRegistry(fixedClock(), diag.DefaultLimits(), runtime),
		"wired nil":   diag.NewRegistry(fixedClock(), diag.DefaultLimits(), runtime).WithAudit(nil),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotPanics(t, func() {
				registry.Catalog()
				_, err := registry.Snapshot(t.Context(), "")
				require.NoError(t, err)
				_, err = registry.Explain(t.Context(), diag.CategoryRuntime, "version")
				require.NoError(t, err)
				_, err = registry.Snapshot(t.Context(), "no-such-category")
				require.Error(t, err)
			})
		})
	}
}

// A caller that stopped waiting is not a fault. It is recorded as its own
// result so a log full of canceled turns cannot be mistaken for a log full
// of broken owners.
func TestACallerThatStoppedWaitingIsNotRecordedAsAFailure(t *testing.T) {
	runtime := &fake{category: diag.CategoryRuntime, status: available("version")}
	registry, log := audited(runtime)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := registry.Snapshot(ctx, diag.CategoryRuntime)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, diag.AuditCanceled, log.only(t).Result)
	assert.Equal(t, 0, runtime.collected, "and nothing was observed on the way out")

	registry, log = audited(runtime)
	_, err = registry.Explain(ctx, diag.CategoryRuntime, "version")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, diag.AuditCanceled, log.only(t).Result)
}

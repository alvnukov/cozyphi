package sessions_test

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func activeID(t *testing.T, r *sessions.Registry) string {
	t.Helper()
	e, ok := r.Active()
	if !ok {
		t.Fatal("expected active session")
	}
	return e.ID
}

func open(t *testing.T, r *sessions.Registry, name string) string {
	t.Helper()
	id, err := r.Open(name, &sessions.View{})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMembershipAndSnapshots(t *testing.T) {
	events := 0
	r := sessions.NewRegistry(3, func() { events++ })
	view := &sessions.View{}
	a, err := r.Open("a", view)
	if err != nil {
		t.Fatal(err)
	}
	b := open(t, r, "b")
	if a == "" || b == "" || a == b || r.Len() != 2 || activeID(t, r) != a {
		t.Fatal("invalid IDs, membership, or initial selection")
	}
	entries := r.Entries()
	if entries[0].Name != "a" || entries[0].View != view || entries[1].ID != b {
		t.Fatal("entries do not preserve insertion order and view identity")
	}
	if entries[0].LastViewed.IsZero() || !entries[1].LastViewed.IsZero() {
		t.Fatal("only selected entries should have a viewing timestamp")
	}
	original := entries[0]
	entries[0] = sessions.Entry{}
	active, _ := r.Active()
	active.Name = "changed"
	if got, _ := r.Active(); got != original {
		t.Fatal("snapshot mutation reached registry")
	}
	if err := r.Activate(a); err != nil || events != 2 {
		t.Fatal("same-session selection should be a no-op")
	}
	if got, _ := r.Active(); got != original {
		t.Fatal("no-op changed timestamp")
	}
	if err := r.Activate(b); err != nil {
		t.Fatal(err)
	}
	if r.Entries()[0] != original || r.Entries()[1].LastViewed.IsZero() || events != 3 {
		t.Fatal("selection changed unrelated entry or missed event")
	}
}

func TestLimitsAndRejectedActions(t *testing.T) {
	for _, limit := range []int{0, -1, 2} {
		capacity := limit
		if capacity <= 0 {
			capacity = 12
		}
		events := 0
		r := sessions.NewRegistry(limit, func() { events++ })
		if _, err := r.Open("nil", nil); err == nil {
			t.Fatal("nil view accepted")
		}
		view := &sessions.View{}
		if _, err := r.Open("first", view); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Open("duplicate", view); err == nil {
			t.Fatal("duplicate view accepted")
		}
		for i := 1; i < capacity; i++ {
			open(t, r, "same name is allowed")
		}
		if _, err := r.Open("overflow", &sessions.View{}); err == nil || !strings.Contains(err.Error(), "close") {
			t.Fatal("limit error must suggest closing a session")
		}
		for _, err := range []error{r.Activate("missing"), r.Jump(0), r.Jump(capacity + 1), r.Back()} {
			if err == nil {
				t.Fatal("invalid selection accepted")
			}
		}
		if v, err := r.Close("missing"); err == nil || v != nil {
			t.Fatal("unknown close accepted")
		}
		if events != capacity || r.Len() != capacity {
			t.Fatal("failed operation changed membership or emitted an event")
		}
	}
}

func TestNavigationAndBackStack(t *testing.T) {
	r := sessions.NewRegistry(4, nil)
	a, b, c := open(t, r, "a"), open(t, r, "b"), open(t, r, "c")
	steps := []struct {
		run  func() error
		want string
	}{
		{r.Prev, c},
		{r.Next, a},
		{r.Next, b},
		{func() error { return r.Jump(3) }, c},
		{func() error { return r.Activate(a) }, a},
		{r.Back, c},
		{r.Back, b},
	}
	for _, step := range steps {
		if err := step.run(); err != nil || activeID(t, r) != step.want {
			t.Fatalf("navigation: got %s, want %s, error %v", activeID(t, r), step.want, err)
		}
	}
	if err := r.Back(); err == nil {
		t.Fatal("back stack should contain no duplicates")
	}
}

func TestCloseAndStableIdentity(t *testing.T) {
	events := 0
	r := sessions.NewRegistry(4, func() { events++ })
	a, b, c := open(t, r, "a"), open(t, r, "b"), open(t, r, "c")
	before := r.Entries()
	if err := r.Activate(c); err != nil {
		t.Fatal(err)
	}
	if v, err := r.Close(c); err != nil || v != before[2].View || activeID(t, r) != a {
		t.Fatal("active close should return view and select latest live visited")
	}
	if _, err := r.Close(a); err != nil || activeID(t, r) != b {
		t.Fatal("active close should fall back to adjacent entry")
	}
	if entry, _ := r.Active(); entry.ID != before[1].ID || entry.Accent != before[1].Accent {
		t.Fatal("close changed retained identity or accent")
	}
	if _, err := r.Close(b); err != nil || r.Len() != 0 {
		t.Fatal("last close failed")
	}
	if _, ok := r.Active(); ok || events != 7 {
		t.Fatal("empty registry still active or incorrect events")
	}
	if id, err := r.Open("reopened", before[0].View); err != nil || id == a {
		t.Fatal("reopened view should get a new live ID")
	}
}

func TestClosedHistoryAndAdjacentFallback(t *testing.T) {
	r := sessions.NewRegistry(4, nil)
	a, b, c := open(t, r, "a"), open(t, r, "b"), open(t, r, "c")
	if err := r.Activate(b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Close(a); err != nil || activeID(t, r) != b {
		t.Fatal("inactive close changed selection")
	}
	if err := r.Back(); err == nil {
		t.Fatal("closed entry remained in history")
	}
	if _, err := r.Close(b); err != nil || activeID(t, r) != c {
		t.Fatal("adjacent fallback failed")
	}
}

func TestEmptyAndSingleSessionOperations(t *testing.T) {
	events := 0
	r := sessions.NewRegistry(1, func() { events++ })
	for _, err := range []error{r.Next(), r.Prev(), r.Jump(1), r.Back(), r.Activate("missing")} {
		if err == nil || err.Error() == "" {
			t.Fatal("empty operation needs an actionable error")
		}
	}
	if _, err := r.Close("missing"); err == nil || events != 0 {
		t.Fatal("empty close should fail without event")
	}
	open(t, r, "only")
	before, _ := r.Active()
	for _, err := range []error{r.Next(), r.Prev(), r.Jump(1)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if after, _ := r.Active(); after != before || events != 1 {
		t.Fatal("single-session cycling must be a no-op")
	}
}

func TestCallbackObservesCommittedState(t *testing.T) {
	var r *sessions.Registry
	var observed []string
	r = sessions.NewRegistry(2, func() {
		if e, ok := r.Active(); ok {
			observed = append(observed, e.ID)
		} else {
			observed = append(observed, "")
		}
	})
	a := open(t, r, "a")
	b := open(t, r, "b")
	if err := r.Activate(b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Close(b); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Close(a); err != nil {
		t.Fatal(err)
	}
	want := []string{a, a, b, a, ""}
	if len(observed) != len(want) {
		t.Fatal("incorrect callback count")
	}
	for i := range want {
		if observed[i] != want[i] {
			t.Fatal("callback saw uncommitted state")
		}
	}
}

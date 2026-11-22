package funcadvisor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

type catalog struct {
	packages []funclib.Package
	private  map[string][]funclib.Package
}

func (c *catalog) Inventory(_ context.Context, _ string, owner string) ([]funclib.Package, []funclib.Package, error) {
	return c.packages, c.private[owner], nil
}
func (c *catalog) Load(_ context.Context, digest string) (funclib.Package, error) {
	for _, p := range c.packages {
		if p.Digest == digest {
			return p, nil
		}
	}
	for _, group := range c.private {
		for _, p := range group {
			if p.Digest == digest {
				return p, nil
			}
		}
	}
	return funclib.Package{}, errors.New("absent")
}

func builds() funclib.Package {
	return funclib.Package{Name: "builds", Version: "1.2.0", Digest: "immutable-1",
		Source: `// export function imaginary() {}
const documentation = "export function invisible() {}";
/** Returns unique package names for failed builds. */
export function failedPackages(rows: Build[]): string[] { return []; }
/** Counts every failed attempt, including retries. */
export const countFailuresByPackage = (rows: Build[]) => new Map();
`, Tests: `test("deduplicates retry failures", () => equal(failedPackages(rows), ["ui"]));
test("counts repeated failures", () => equal(countFailuresByPackage(rows).size, 1));`,
	}
}

func decodeHints(t *testing.T, s string) []Card {
	t.Helper()
	var cs []Card
	for s := range strings.SplitSeq(s, "\n") {
		if s == "" {
			continue
		}
		var c Card
		if err := json.Unmarshal([]byte(s), &c); err != nil {
			t.Fatal(err)
		}
		cs = append(cs, c)
	}
	return cs
}

func TestRecallFindsActualExportsInRussianAndIncludesTest(t *testing.T) {
	selector := New(nil, nil, 0)
	cat := &catalog{packages: []funclib.Package{builds()}}
	selector.store = cat
	hints := decodeHints(t, selector.Hints(t.Context(), "session", "/work", "выведи пакеты с упавшими сборками", nil))
	if len(hints) == 0 || hints[0].Name != "failedPackages" || hints[0].Test != "deduplicates retry failures" ||
		!strings.Contains(hints[0].Use, `from "cozy:lib/builds"`) || hints[0].Version != "1.2.0" {
		t.Fatalf("hints %+v", hints)
	}
	all := Search(cat.packages, "", 64)
	if len(all) != 2 {
		t.Fatalf("parsed declarations from docs or skipped a callable: %+v", all)
	}
	if !strings.Contains(all[0].Signature, "Build[]") {
		t.Fatal(all)
	}
}

func TestSelectionReconcilesChangedLocksAndPrivateDrafts(t *testing.T) {
	released := builds()
	draft := funclib.Package{Name: "wip", Source: "export function hiddenWork() {}", Digest: "draft-1"}
	cat := &catalog{packages: []funclib.Package{released}, private: map[string][]funclib.Package{"one": {draft}}}
	s := New(nil, nil, 0)
	s.store = cat
	if got := s.Hints(t.Context(), "two", "/w", "hiddenWork", nil); got != "" {
		t.Fatal(got)
	}
	if hints := decodeHints(t, s.Hints(t.Context(), "one", "/w", "hiddenWork", nil)); len(hints) != 1 || !hints[0].Draft {
		t.Fatal(hints)
	}
	before := s.Hints(t.Context(), "two", "/w", "failedPackages", nil)
	cat.packages[0].Version = "1.3.0"
	cat.packages[0].Digest = "immutable-2"
	after := s.Hints(t.Context(), "two", "/w", "failedPackages", nil)
	if !strings.Contains(before, "immutable-1") || !strings.Contains(after, "immutable-2") {
		t.Fatal(before, after)
	}
}

func TestAdvisorAbstainsFallsBackOrRefines(t *testing.T) {
	for _, tc := range []struct {
		name  string
		v     vote
		fails bool
		want  string
	}{
		{"empty", vote{IDs: []string{}}, false, ""},
		{"unknown", vote{IDs: []string{"invented"}}, true, "failedPackages"},
		{"invalid", vote{}, true, "failedPackages"},
		{"reformulate", vote{IDs: []string{}, Query: "countFailuresByPackage"}, false, "countFailuresByPackage"},
		{"one", vote{IDs: []string{"builds/countFailuresByPackage"}}, false, "countFailuresByPackage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewWithPolicy(nil, nil, 0, Always)
			s.store = &catalog{packages: []funclib.Package{builds()}}
			s.advise = func(context.Context, string, []Card) (vote, llm.Usage, error) {
				return tc.v, llm.Usage{PromptTokens: 12, CompletionTokens: 3}, nil
			}
			h := s.Hints(t.Context(), "s", "/work", "failed packages and counts", nil)
			if tc.want == "" && h != "" || tc.want != "" && !strings.Contains(h, tc.want) {
				t.Fatal(h)
			}
			stats := s.Stats()
			if stats.Calls != 1 || stats.InputTokens != 12 || (stats.Fallbacks == 1) != tc.fails {
				t.Fatalf("%+v", stats)
			}
			_ = s.Hints(t.Context(), "s", "/work", "failed packages and counts", nil)
			if !tc.fails && s.Stats().Calls != 1 {
				t.Fatal("a valid identical selection missed the cache")
			}
		})
	}
}

func TestExactMatchSkipsAdvisorAndDeadlineKeepsLocalResults(t *testing.T) {
	s := New(nil, nil, time.Millisecond)
	s.store = &catalog{packages: []funclib.Package{builds()}}
	s.advise = func(ctx context.Context, _ string, _ []Card) (vote, llm.Usage, error) {
		<-ctx.Done()
		return vote{}, llm.Usage{}, ctx.Err()
	}
	if h := s.Hints(t.Context(), "s", "/work", "use failedPackages", nil); !strings.Contains(h, "failedPackages") || s.Stats().Calls != 0 {
		t.Fatal(h)
	}
	h := s.Hints(t.Context(), "s", "/work", "failed packages and counts", nil)
	if h == "" || s.Stats().Fallbacks != 1 {
		t.Fatal(h, s.Stats())
	}
}

type roundtrip func(*http.Request) (*http.Response, error)

func (f roundtrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAdvisorCallsConfiguredModelThroughNormalProtocol(t *testing.T) {
	t.Setenv("COZYPHI_TEST_API_TOKEN", "private-value-in-goal")
	c := util.DefaultHTTPClient()
	prev := c.Transport
	c.Transport = roundtrip(func(req *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		body := string(raw)
		if strings.Contains(body, "private-value-in-goal") || strings.Contains(body, "PRIVATE_PASTED_CODE") || strings.Contains(body, `"tools":[{`) {
			return nil, errors.New("request contains excess data")
		}
		if !strings.Contains(body, "side-model") || !strings.Contains(body, "builds/countFailuresByPackage") {
			return nil, errors.New("missing candidate or route")
		}
		const event = `data: {"choices":[{"delta":{"content":"{\"ids\":[\"builds/countFailuresByPackage\"]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":23,"completion_tokens":4}}

data: [DONE]

`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(event)), Request: req}, nil
	})
	t.Cleanup(func() { c.Transport = prev })
	s := NewWithPolicy(nil, &llm.ModelConfig{Name: "side-model", BaseURL: "http://advisor.invalid"}, time.Second, Always)
	s.store = &catalog{packages: []funclib.Package{builds()}}
	h := decodeHints(t, s.Hints(t.Context(), "s", "/w", "failed packages and counts private-value-in-goal ```PRIVATE_PASTED_CODE", nil))
	if len(h) != 1 || h[0].Name != "countFailuresByPackage" || s.Stats().InputTokens != 23 || s.Stats().Fallbacks != 0 {
		t.Fatal(h, s.Stats())
	}
}

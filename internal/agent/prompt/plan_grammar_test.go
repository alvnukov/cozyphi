package prompt

import (
	"strings"
	"testing"
)

// grammarBlockStart marks the authoring grammar block in plan-prompt.tmpl;
// everything from this line to the end of the template is the grammar.
const grammarBlockStart = "Authoring grammar:"

// approxTokens maps word count to a token estimate. English prose runs about
// four tokens per three words on the cl100k-family tokenizers, so a word band
// is a faithful dependency-free encoding of the roughly 130-170 token budget.
func approxTokens(words int) int { return words * 4 / 3 }

func planGrammarBlock(t *testing.T) string {
	t.Helper()
	idx := strings.Index(planPromptTmpl, grammarBlockStart)
	if idx < 0 {
		t.Fatal("expected authoring grammar block in plan-prompt.tmpl")
	}
	return planPromptTmpl[idx:]
}

func TestPlanPromptGrammarWithinBudget(t *testing.T) {
	words := len(strings.Fields(planGrammarBlock(t)))
	if words < 95 || words > 135 {
		t.Fatalf("authoring grammar block is %d words (~%d tokens); want 95-135 words (~130-170 tokens)", words, approxTokens(words))
	}
}

func TestPlanPromptGrammarCoversContract(t *testing.T) {
	block := planGrammarBlock(t)
	for _, concept := range []string{"obligation", "workstream", "dependenc", "uncertaint", "evidence", "smallest complete bespoke", "capability type", "Self-check", "coverage", "observability", "mergeability", "risk"} {
		if !strings.Contains(block, concept) {
			t.Fatalf("authoring grammar must cover %q", concept)
		}
	}
	if !strings.Contains(block, "judgement, not a harness validator") {
		t.Fatal("self-check must read as model-side judgement, not a harness validator")
	}
}

func TestPlanPromptGrammarHasNoHiddenSemantics(t *testing.T) {
	for _, forbidden := range []string{"archetype", "role", "Model", "Actions"} {
		if strings.Contains(planPromptTmpl, forbidden) {
			t.Fatalf("plan prompt must not mention %q", forbidden)
		}
	}
}

func TestBuildPlanAppendixCarriesGrammar(t *testing.T) {
	if !strings.Contains(Build(Options{Plan: true}), grammarBlockStart) {
		t.Fatal("expected authoring grammar to reach the plan-mode prompt")
	}
	if strings.Contains(Build(Options{}), grammarBlockStart) {
		t.Fatal("authoring grammar must not leak into the build prompt")
	}
}

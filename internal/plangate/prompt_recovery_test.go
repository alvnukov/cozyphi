package plangate_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/plangate"
)

func TestPromptBlockTeachesHostToolBindingProtocol(t *testing.T) {
	for _, phase := range []plangate.Phase{plangate.PhaseDeny, plangate.PhaseHint} {
		t.Run(string(phase), func(t *testing.T) {
			block := strings.Join(strings.Fields(plangate.PromptBlock(phase)), " ")
			for _, instruction := range []string{
				"Cozyphi tools are not generic tools",
				"every non-exempt call must include plan_step, even for familiar names",
				"each non-exempt child carries its own plan_step",
				"Auto-binding is recovery; still pass plan_step",
				"After a binding refusal, correct plan_step and retry the same tool",
				"not a different pattern, path or tool",
				"If unsure, use plan action get",
			} {
				assert.Contains(t, block, instruction)
			}
		})
	}
}

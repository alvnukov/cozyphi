package plantool

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/session"
)

// Model remains a canonical user field. Inspect presence at the tool boundary
// so null and empty authoring attempts cannot disappear into string zero values.
// Only the selected action/op owns intent; provider-materialized siblings do not.
func rejectModelAuthoring(raw json.RawMessage, in input) error {
	var payload struct {
		Steps []map[string]json.RawMessage `json:"steps"`
		Ops   []struct {
			Op   string                     `json:"op"`
			Step map[string]json.RawMessage `json:"step"`
		} `json:"ops"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("plan args: %w", err)
	}
	hasModel := func(fields map[string]json.RawMessage) bool {
		for key := range fields {
			if strings.EqualFold(key, "model") {
				return true
			}
		}
		return false
	}
	switch in.Action {
	case "create", "update", "":
		if slices.ContainsFunc(payload.Steps, hasModel) {
			return errHumanOnly
		}
	case "patch":
		for i, op := range payload.Ops {
			switch op.Op {
			case session.PlanPatchUpdateStep:
				if in.Ops[i].Model.Set {
					return errHumanOnly
				}
			case session.PlanPatchInsertStep, session.PlanPatchSupersedeStep:
				if hasModel(op.Step) {
					return errHumanOnly
				}
			}
		}
	}
	return nil
}

func normalizeInputEfforts(in *input) error {
	normalize := func(value *string) error {
		effort, err := session.NormalizePlanEffort(*value)
		if err != nil {
			return err
		}
		*value = effort
		return nil
	}
	for i := range in.Steps {
		if err := normalize(&in.Steps[i].Effort); err != nil {
			return err
		}
	}
	for i := range in.Ops {
		op := &in.Ops[i]
		if op.Effort.Set {
			if err := normalize(&op.Effort.Value); err != nil {
				return err
			}
		}
		if op.Step != nil {
			if err := normalize(&op.Step.Effort); err != nil {
				return err
			}
		}
	}
	return nil
}

package agenttool

import (
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm/skills"
)

// Bounds on one explicit skill selection: enough breadth for a sub-task,
// small enough that the rendered child prompt stays cheap. Bodies, not
// names, are what the child prompt carries verbatim.
const (
	maxSpawnSkills     = 8
	maxSpawnSkillBytes = 32 * 1024
)

// resolveSpawnSkills turns the model's skills decision into the canonical
// names the spawned job carries. Every requested name must resolve against
// the installed catalog — an unknown name fails closed with the catalog
// listing, so one retry is enough to correct it. Duplicates fold
// case-insensitively, keeping the first occurrence's order.
func resolveSpawnSkills(skillPath string, requested []string) ([]string, error) {
	catalog, err := skills.LoadSkills(skillPath)
	if err != nil {
		return nil, fmt.Errorf(
			"agent_spawn: cannot load skills from %s: %w — fix the directory, or pass skills: [] with no_skill_reason",
			skillPath, err,
		)
	}
	var (
		seen      = make(map[string]struct{}, len(requested))
		names     = make([]string, 0, len(requested))
		bodyBytes int
	)
	for _, name := range requested {
		skill := skills.Find(catalog, name)
		if skill == nil {
			return nil, unknownSkillError(skillPath, catalog, name)
		}
		key := strings.ToLower(skill.Name)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, skill.Name)
		bodyBytes += len(skill.Body)
		if len(names) > maxSpawnSkills {
			return nil, fmt.Errorf(
				"agent_spawn: at most %d skills per spawn, got %d — keep only the ones this sub-task needs",
				maxSpawnSkills, len(names),
			)
		}
		if bodyBytes > maxSpawnSkillBytes {
			return nil, fmt.Errorf(
				"agent_spawn: skill bodies total %d bytes, over the %d-byte limit — pass fewer or smaller skills",
				bodyBytes, maxSpawnSkillBytes,
			)
		}
	}
	return names, nil
}

// unknownSkillError names the miss and the way out: the catalog listing when
// skills exist, the explicit none-installed answer when they do not.
func unknownSkillError(skillPath string, catalog []*skills.Skill, name string) error {
	if len(catalog) == 0 {
		return fmt.Errorf(
			"agent_spawn: unknown skill %q and no skills are installed in %s — pass skills: [] with no_skill_reason explaining why",
			name,
			skillPath,
		)
	}
	available := make([]string, 0, len(catalog))
	for _, skill := range catalog {
		available = append(available, skill.Name)
	}
	return fmt.Errorf(
		"agent_spawn: unknown skill %q — installed skills: %s",
		name, strings.Join(available, ", "),
	)
}

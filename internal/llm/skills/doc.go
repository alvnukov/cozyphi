// Package skills loads and parses SKILL.md files from skill directories.
//
// Each skill is a directory containing a SKILL.md file with YAML frontmatter
// and a Markdown body. Skills are loaded from ~/.phi/skills/ (or a
// custom path set via skill_path in config or PHI_SKILL_PATH env var).
//
// A skill file looks like:
//
//	---
//	name: My Skill
//	description: What this skill does
//	---
//	Instructions for the agent to follow when this skill is relevant.
package skills

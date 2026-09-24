// Package skills loads and parses SKILL.md files from skill directories.
//
// Each skill is a directory containing a SKILL.md file with YAML frontmatter
// and a Markdown body. Skills are loaded from ~/.cozyphi/skills/ (or a
// custom path set via skill_path in config or COZYPHI_SKILL_PATH env var).
//
// A skill file looks like:
//
//	---
//	name: My Skill
//	description: What this skill does
//	---
//	Instructions for the agent to follow when this skill is relevant.
//
// The catalog is a Sources list: skill_path plus, when Claude Code plugins
// are enabled, one Source per plugin skill directory. A plugin source
// namespaces every skill it finds as "<plugin>:<name>", so a plugin's
// "brainstorming" and a user's own "brainstorming" coexist as
// "superpowers:brainstorming" and "brainstorming". Sources.Load re-reads
// disk on every call and follows directory symlinks, guarding against a
// cycle by each directory's resolved real path, so a link back up the tree
// (or a dangling link) stops that branch without stopping the rest of the
// walk. Find resolves a name in three passes: an exact match, then a
// case-insensitive match, then a bare name — the part after ":", or the
// skill's directory base name — that names exactly one skill; an ambiguous
// bare name is an error listing every candidate.
package skills

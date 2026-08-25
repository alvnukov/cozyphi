// Package project provides the cozyphi workspace layout and configuration.
//
// Discover creates the global cozyphi home (~/.cozyphi) with its standard
// subdirectories (bin, skills, hooks, session, jobs) so downloaded tool
// binaries, SKILL.md files, hook manifests, and persisted sessions have a
// known home. This mirrors panda's internal/project: startup ensures the
// layout exists, then tools such as fd/ripgrep are downloaded into the bin
// directory when missing.
package project

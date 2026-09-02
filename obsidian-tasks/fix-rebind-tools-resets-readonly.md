---
id: fix-rebind-tools-resets-readonly
title: rebindTools молча сбрасывает кастомный список тулов в DefaultTools
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - SetModel/SetJobs на readonly-движке сохраняет readonly
    - регрессионный тест
verification_plan:
    - go test ./internal/agent/...
created_at: "2026-08-23T15:17:22.106276Z"
updated_at: "2026-08-23T16:13:27.069346Z"
---

## Body

internal/agent/engine.go: rebindTools() called buildToolList(nil), so the base list fell back to DefaultTools(); after SetModel/SetJobs, an engine assembled with Tools: ReadonlyTools() silently received write/edit. Fix (commit 7d3ee13): the base tool set is stored in the engine field baseTools (EngineOpts.Tools, nil = DefaultTools), buildToolList became a no-argument method and rebuilds from baseTools; rebindTools now does not expand the set. Regression tests TestSetModelKeepsCustomTools / TestSetJobsKeepsCustomTools (engine_tools_test.go) on the public seam NewEngine → setter → HasTool. Gates: fmt-check/lint/test green.

## Acceptance Criteria

- SetModel/SetJobs на readonly-движке сохраняет readonly
- регрессионный тест

## Verification Plan

1. go test ./internal/agent/...

---
id: refactor-prompt-snapshot
title: 'prompt.Build: чистый snapshot вместо скрытого IO и panic в конструкторе'
status: todo
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - NewEngine без IO и panic
    - промпт ребёнка видит свой WorkDir
    - тест промпта без файловой системы
verification_plan:
    - go test ./internal/agent/prompt/...
created_at: "2026-08-23T15:17:22.11967Z"
updated_at: "2026-08-23T15:17:22.11967Z"
---

## Body

prompt.go:105-111 panic на os.Getwd ошибке; execTmpl паникует; на каждый вызов — обход предков за AGENTS.md/CLAUDE.md и чтение ~/.phi. Зовётся из NewEngine, rebindTools (дважды на смену модели) и каждого спавна. Плюс formatProjectContext (context.go:98) интерполирует путь в псевдо-XML без эскейпинга, и промпт ребёнка штампуется process-cwd вместо session WorkDir. Кандидат: Build(snapshot{cwd, root, files, skills, servers}) — hermetic, кэшируемый, тестируемый; NewEngine перестаёт паниковать.

## Acceptance Criteria

- NewEngine без IO и panic
- промпт ребёнка видит свой WorkDir
- тест промпта без файловой системы

## Verification Plan

1. go test ./internal/agent/prompt/...

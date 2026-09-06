---
id: execution-control-write-approval
title: 'Запись: требовать согласия на изменение Git config и hooks'
status: todo
priority: critical
model_level: medium
task_type: bug
parent_id: remove-executable-default-bash-allowlist
tags:
    - security
    - permissions
acceptance_criteria:
    - Изменение Git-конфигурации и Git hooks через штатные инструменты записи не получает автоматического разрешения обычной workspace-записи; требуется явное согласие.
    - Проверка учитывает канонический путь, ссылки и вынесенный Git directory в worktree; неизвестный target не считается безопасным.
    - Обычные исходники остаются доступны по прежней политике; существующие deny-правила и точное связывание разрешения не ослаблены.
    - 'Тесты покрывают цепочку подготовки управляющего файла перед последующей командой: без согласия запись не происходит, внешние процессы не запускаются.'
    - Точный защищаемый набор и ограничения явно описаны; изменение поведения отражено в Unreleased.
verification_plan:
    - Проверить через LSP общую границу permission для write/edit и текущую классификацию чувствительных путей.
    - В temp fixtures проверить обычный репозиторий, worktree с вынесенным Git directory, относительные пути и symlink; не менять управляющие файлы рабочего репозитория.
    - Проверить refusal, отмену и явное согласие через fake executor; подтвердить отсутствие записи и запуска hook до согласия.
    - Запустить адресные permission/write/edit регрессии; проверить changelog и diff. Не выполнять общий suite или lint без согласования.
created_at: "2026-09-06T09:25:17.476226Z"
updated_at: "2026-09-06T09:25:17.476226Z"
---

## Body

**Родитель:** remove-executable-default-bash-allowlist.

**Результат:** разрешение агенту редактировать исходники не даёт незаметно менять Git-конфигурацию или hooks, влияющие на последующее исполнение.

**Границы:** минимальный набор — Git config и hooks; использовать существующую классификацию чувствительных путей и общую permission boundary для write/edit. Сначала проверить актуальную реализацию: если случаи уже закрыты, добавить недостающие регрессии и evidence вместо нового слоя. Не пытаться запретить все исполняемые исходники или превратить gate в sandbox. Защита записи через произвольный разрешённый shell — не гарантия этой задачи; общий shell parsing вынесен отдельно. MCP trust не входит.

**Блокируется:** нет — независима от изменения Bash defaults.

**Исполнение:** model_level medium; делегирование только с явно заданным medium или ниже. Код в task worktree, native task для main-реестра, MCP не использовать. При необходимости расширить перечень файлов или permission API сначала согласовать границы; lint без нового разрешения не запускать.

## Acceptance Criteria

- Изменение Git-конфигурации и Git hooks через штатные инструменты записи не получает автоматического разрешения обычной workspace-записи; требуется явное согласие.
- Проверка учитывает канонический путь, ссылки и вынесенный Git directory в worktree; неизвестный target не считается безопасным.
- Обычные исходники остаются доступны по прежней политике; существующие deny-правила и точное связывание разрешения не ослаблены.
- Тесты покрывают цепочку подготовки управляющего файла перед последующей командой: без согласия запись не происходит, внешние процессы не запускаются.
- Точный защищаемый набор и ограничения явно описаны; изменение поведения отражено в Unreleased.

## Verification Plan

1. Проверить через LSP общую границу permission для write/edit и текущую классификацию чувствительных путей.
2. В temp fixtures проверить обычный репозиторий, worktree с вынесенным Git directory, относительные пути и symlink; не менять управляющие файлы рабочего репозитория.
3. Проверить refusal, отмену и явное согласие через fake executor; подтвердить отсутствие записи и запуска hook до согласия.
4. Запустить адресные permission/write/edit регрессии; проверить changelog и diff. Не выполнять общий suite или lint без согласования.

## Progress

**2026-09-06:** реализовано в общем permission-шве без нового слоя.

- `Policy.ControlPathAsk []string` — nil означает «без мнения», и гейт
  выводит префиксы сам на каждой проверке (`defaultControlPaths` в rules.go,
  `controlPrefixes` в gate.go): root `.git` каталог → `config`, `hooks`,
  `worktrees`; `.git`-файл-указатель → сам указатель + gitdir/commondir
  через `commondir` (config, hooks, worktrees общего каталога); нечитаемый
  указатель → fail-closed на префикс `.git`. Живой вывод покрывает
  `git init` после старта сессии; политика вызывающего не мутируется.
- `checkPaths`: после sensitive-deny и workspace-deny (порядок: deny сильнее
  согласия) write/edit на control-путь → `Ask` с причиной «git control
  file requires approval»; reads и `.git/index`/refs не тронуты.
- Executor: согласие Ask привязано к пути и передано в mutation guard —
  re-check mid-write принимает тот же Ask на тот же путь и ничего другое.
- Ограничения: `core.hooksPath` вне скоупа (правка config уже под Ask);
  указатель worktree — контроль для гейта самого worktree, из главного
  гейта это обычный файл; путь под файлом-указателем не резолвится → Deny.
- Тесты: `TestWriteGitControlFilesAsk` (root/symlink/relative/allow-пины),
  `TestWriteGitControlFilesAskAcrossWorktrees` (common dir, worktrees,
  указатели, внешний gitdir из worktree → Deny),
  `TestGitControlWriteConsentChain` (real gate + writetool: без согласия
  файла нет, со согласием записан). `go test ./...` зелёный, gofmt чист.
- Ревью (Standards + Spec): закрыт gap «gate снапшотил layout при
  construction» (теперь per-check, тест LateGitInit), `ControlPathAsk`
  закреплён тестом-override, `IsSensitivePath` → нейтральный
  `matchesPrefix`, именованные результаты checkPermission, CHANGELOG
  перенесён в Security. `.git/worktrees` целиком — согласованное решение.

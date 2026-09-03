---
id: claude-modes-and-brief
title: Режимы как данные, сборка брифа с бюджетом, типизированный разбор ответа
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - modes
    - brief
acceptance_criteria:
    - Preset каждого режима задокументирован в коде и покрыт golden-тестом argv-релевантных полей; modes override из claude.json применяется и fail-closed на неизвестных ключах.
    - Render уважает лимиты выдержки и брифа, отказывает с именем файла и подсказкой lines, не читает файлы вне workspace.
    - ParseAnswer разбирает фикстуры всех шести схем и возвращает ошибку с ограниченным raw-хвостом на мусор.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/claude/... -run 'Brief|Preset|Answer' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.87363Z"
updated_at: "2026-09-02T05:13:31.87363Z"
---

## Body

**Цель.** Всё, что делает вызов эффективным, живёт в данных: пресет режима и правила брифа.

**Режимы (Mode).** architect, review_plan, review_diff, diagnose, research, implement. Preset{SystemAddendum, Tools, AllowedTools, DisallowedTools, PermissionMode, Effort, Model, Schema, Timeout, SettingSources, StrictMCP, DisableSlashCommands, NoSessionPersistence, AllowsFS}. Дефолты: architect/review_plan/review_diff/diagnose — без инструментов (Tools пуст), StrictMCP, DisableSlashCommands, SettingSources пустой, effort high, timeout 180s; research — Tools Read,Grep,Glob, AddDirs=workdir, effort high, timeout 600s; implement — профиль задаётся в задаче claude-implement-isolation, здесь только заглушка с AllowsFS=true. Model по умолчанию пустая (дефолт claude); конфиг claude.json получает modes: {<mode>: {model, effort, timeout_sec}} с теми же правилами fail-closed (расширение конфига из claude-runner-proc-adapter).

**Схемы ответов (JSON Schema, --json-schema).** architect: {decision, options[{name, pros, cons}], recommendation, risks[], open_questions[]}; review_plan: {verdict: approve|revise|reject, findings[{step_id, severity: must|should|nit, problem, fix}], missing_steps[], risks[]}; review_diff: {verdict, findings[{path, line, severity, problem, fix}], missing_tests[]}; diagnose: {root_cause, evidence[], fix, verify}; research: {answer, evidence[{path, lines, note}], confidence, unknowns[]}; implement: {summary, files_changed[], verification, notes}.

**Бриф.** Brief{Task, Deliverable, Context, Files []FileRef{Path, From, To}, Attachments []Attachment{Kind, Title, Body}} → Render(mode, workdir) (prompt, error). Файлы читаются относительно workdir, обязаны быть внутри workspace (permission.InWorkspace, без symlink-побега — существующие path-хелперы); выдержка ≤ 16 KB, весь бриф ≤ 64 KB; превышение — отказ, называющий файл, размер, лимит и подсказку lines. Форма промпта: преамбула («тебя консультирует другой агент; ты не видишь его сессию; отвечай только по схеме»), выдержки в fenced-блоках с заголовком path:from-to, вложения по видам, затем Task, Context, Deliverable. Лимиты — константы, из которых описание тулы потом рендерит цифры (образец watchtool).

**Разбор ответа.** ParseAnswer(mode, Result) (Answer, error): структуры на каждую схему, вход — StructuredOutput, при его отсутствии — попытка разобрать Text как JSON; кривой ответ — ошибка с ограниченным raw-хвостом, никогда не panic. Answer.Render() — компактный текст для tool result (findings построчно с severity и step_id/path:line).

**Тесты.** golden-брифы на каждый режим, отказ по бюджету, побег из workspace, разбор фикстур ответов и мусора, конфиг modes override.

**Зависит от:** claude-runner-proc-adapter (типы Request/Result, конфиг).

## Acceptance Criteria

- Preset каждого режима задокументирован в коде и покрыт golden-тестом argv-релевантных полей; modes override из claude.json применяется и fail-closed на неизвестных ключах.
- Render уважает лимиты выдержки и брифа, отказывает с именем файла и подсказкой lines, не читает файлы вне workspace.
- ParseAnswer разбирает фикстуры всех шести схем и возвращает ошибку с ограниченным raw-хвостом на мусор.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/claude/... -run 'Brief|Preset|Answer' -race.
2. make fmt-check lint test.

---
id: web-tools
title: Protected asynchronous web research and quarantine
status: in_progress
priority: high
model_level: high
task_type: epic
parent_id: cozy-tools-extraction
tags:
    - tools
    - security
    - cozy-tools
acceptance_criteria:
    - Preserve the shipped cozy-tools web extraction, published dependency and helper integration as the legacy baseline, not proof of the redesigned feature.
    - 'Deliver the confirmed protected asynchronous web research contract in specs/protected-web-research.md: explicit user-selected capable model, parallel quarantined safety/extraction, final candidate screening, checked evidence and immutable reusable sources.'
    - Support public static pages, PDF, isolated JavaScript rendering and visual/OCR research; declare unavailable safe modes instead of silently weakening isolation.
    - Prevent whole-harness bypass and preserve material restrictions across turns, resume, forks, compaction and derivatives; restricted protected mode may disable unmediated tools.
    - Provide user-wide canonical hostname blocking for source-attributed decoy calls, dependency revocation, human-only incident investigation and user-controlled recheck/unblock without unchecked raw delivery.
    - Provide bounded background jobs with origin-bound delivery, explicit resume, protected scoped caches and shared account admission favoring interactive work.
    - 'Meet the agreed finite-corpus acceptance: zero successful release/action/egress bypasses and at most 5% benign-task false stops; report quality, coverage, latency and usage without universal safety claims.'
verification_plan:
    - Use the approved public web/session and lifecycle seams and real process/filesystem/egress boundaries from specs/protected-web-research.md; fake model/network fixtures establish deterministic harness behavior.
    - Exercise safe multi-format acquisition, parallel-release races, decoys, final screening, source reuse, revocation, grants, recipient control and bypass prevention on each supported platform.
    - Run separately authorized finite attack and benign-task evaluations with actual release/action/egress outcomes, false-stop accounting, quality, latency and usage measurements.
    - Keep implementation gates scoped to changed files/packages; specification publication alone does not close this epic.
created_at: "2026-09-07T11:00:00Z"
updated_at: "2026-09-19T16:12:49.249089Z"
---

## Body

**Откуда.** Запрос пользователя 2026-09-07: в helper есть веб-тулы, в козе нет; взять через cozy-tools, сделав безопасными для модели.

**Что уже есть в helper (internal/mcp/web_tools.go, internal/webfetch, internal/websearch, ~1160 строк, только stdlib).** Одна MCP-тула `web` с action search/fetch/find/read. Fetch кладёт source.bin + normalized.txt + metadata.json в кеш по doc_id (sha256 URL+тела), модели отдаёт только метаданные. Read — ограниченный фрагмент, Find — сниппеты с офсетами. Search: duckduckgo_html (регексы по HTML) и google_cse (ключ из env, редакция ключа в ошибках). WebPolicy: enabled, cache_dir, max_source_bytes, timeout, max_redirects, allowed/denied hosts, схемы, content types, UA, провайдер. Защита от SSRF на двух уровнях: validateURL (схема, localhost, непубличный IP, deny/allow hosts) и dial guard по резолвнутому адресу (обходится только при явном allowed_hosts).

**Чего не хватает для «безопасно для модели».** Рамки untrusted вокруг текста страницы (главный риск — prompt injection со страницы); интеграции с permission gate козы; своего кеш-пути и конфига; документации потолка для детей.

**Шаги.** 1 cozy-tools: пакеты webfetch/websearch + WebPolicy (агент в репо cozy-tools). 2 параллельно после 1: helper переезжает на cozy-tools; коза получает internal/tools/webtool + ActionWeb + config + рамка + доки.

**Защита от prompt injection со страниц (решение 2026-09-07, требование пользователя: «максимально безопасно»).** Детекторы-классификаторы ненадёжны (kill-test локальных guard-моделей 2026-09-03 провалили все четыре кандидата), поэтому опора — структурные слои, а модельная проверка — поведенческая, не текстовая. Слои, снизу вверх:
- Нормализация (cozy-tools webfetch). В normalized.txt не попадают HTML-комментарии, `<template>/<noscript>/<head>` (кроме title), элементы с `hidden`, `aria-hidden="true"`, inline display:none / visibility:hidden / font-size:0 / opacity:0; вырезаются zero-width и bidi control символы; source.bin остаётся сырым. Диагностики `hidden_content_stripped` (счётчики) и `low_visible_text`. Эвристика, задокументирована как таковая.
- Бюджет видимости. fetch отдаёт модели только метаданные; read/find — ограниченные фрагменты (4000/20000, 80/500) с офсетами; ссылки не следуются автоматически, каждый URL — отдельный вызов через гейт.
- Карантинный читатель (cozyphi, идея пользователя). По умолчанию основной агент не видит текст страницы: `web read/find` отдают фрагмент tool-less ребёнку роли `web-reader` вместе с вопросом основного агента; ребёнок возвращает выжимку (с дословными цитатами кода по запросу). Ребёнку объявлены **ложные тулы** (декои: bash, write, edit, web fetch с настоящими описаниями из каталога, но без исполнителя). Любой вызов декоя = документ скомпрометирован: вызов не исполняется, doc получает флаг `injection_suspected:<tool>` в metadata.json, основной агент получает короткое уведомление вместо текста, пользователь — тост и строку в транскрипте. Дальнейший read этого doc_id только с явного подтверждения пользователя (`raw`). Ограничения честно: инъекция может не дёргать тулы, а отравлять выжимку; выжимка всё равно приходит в рамке untrusted и помечает ход.
- `raw`-режим. `web read raw:true` отдаёт фрагмент напрямую в рамке untrusted (нужен для точных фрагментов документации); в гейте отдельная деталь, по умолчанию ask даже при гранте на host.
- Рамка untrusted. Любой веб-текст (выжимка, raw, сниппеты search) приходит в обёртке с экранированием как в internal/agent/outcomes.go, чтобы содержимое не могло закрыть рамку; описание тулы говорит то же.
- Помеченный ход (tainted turn). Как только веб-текст попал в ход, мутирующие и egress-действия (bash, write/edit вне worktree проекта, mcp_call, web fetch на новый host, agent spawn с web) требуют явного подтверждения пользователя до конца хода, независимо от прежних грантов; в детали гейта — «after web content in this turn».
- Egress. URL и search-запрос проверяются security.Mask на секреты из окружения/конфига, длина URL ограничена (2 KiB), URL показан в детали гейта и в транскрипте полностью; ask по умолчанию на каждый fetch, грант на host живёт до конца сессии.
- Дети. web в потолке роли; `web-reader` — роль без реальных тулов, не может спавнить.
Реализация: нормализация и флаги doc — cozy-tools (агент уже получил дополнение); читатель, декои, tainted turn, гейт, рамка — cozyphi internal/tools/webtool + internal/agent (фаза после слияния cozy-tools web-family).

**Progress (2026-09-08).** Шаг 1 готов: cozy-tools main e9d27ae (webfetch/websearch/config.WebPolicy, нормализация скрытого контента, Flags+SetFlags, ct-009). Шаг 2 запущен параллельно: helper → cozy-tools web (ветка feature/web-from-cozy-tools в helper), коза → internal/tools/webtool + ActionWeb + карантинный читатель с декоями + tainted turn (ветка feature/web-tools, .worktrees/web-tools). Зависимость на cozy-tools через replace на локальный путь, как в helper; перед релизом козы нужен тег cozy-tools.

**Progress (2026-09-07, ночь).** Половина helper готова: ветка feature/web-from-cozy-tools слита в helper main (6d340fc), internal/webfetch и internal/websearch удалены, WebPolicy — алиас на cozy-tools. По пути на helper main вскрылись два долга от переезда command-семейства (7ca89f3), не от web: bridge.go ссылался на удалённый DefaultConfigPathFn (исправлено 0061e83) и был потерян helper-текст отказа для защищённого конфига (восстановлен через DenyMessage/ProtectedMarkers, 8d39dda). Третий долг — отказ на .lean-реестр — заведён тикетом в helper (lean-registry-denial-lost-in-command-seam), не в этом эпике. Половина козы (feature/web-tools) в работе.

**Progress (2026-09-07, 04:20).** Половина козы слита в main (abafea5, ветка feature/web-tools удалена): internal/tools/webtool (search/fetch/find/read, рамка через JSON-экранирование, egress-маска секретов и лимит URL 2 KiB), permission.ActionWeb + checkWeb (raw всегда ask, allow-list permissions.web.allow по host) + TaintGate поверх всего гейта включая allow-all, карантинный читатель как один Stream-раунд без сессии/спавна с декоями bash/write/edit/web из живого реестра, флаг injection_suspected:<tool> через SetFlags, уведомление WebNotice, секция web в конфиге (opt-in: без блока web: тула выключена), doc/web.md, AGENTS.md, CHANGELOG. Гейты: build cmd ok; go test -race по webtool, permission, agent, project, session, job, tui/controller, tui/overlays, cmd — все ok; lint по затронутым пакетам 0 issues (в ветке). Осталось до закрытия: ручная проверка в сессии (search → fetch → find → read на публичной странице, срабатывание декоя) и тег cozy-tools вместо replace на локальный путь перед релизом.

**Progress (2026-09-07, 15:05).** Тег cozy-tools v0.2.0 опубликован, replace на локальный путь убран из go.mod (тикет cozy-tools-require-tag, PR fix/cozy-tools-require). До закрытия остаётся только ручная проверка в сессии.

**Note (2026-09-19).** Requirements superseded by the user-confirmed 43-decision interview and standalone specification [Protected asynchronous web research](../specs/protected-web-research.md), recorded by protected-web-research-spec. Earlier progress notes saying that only a manual smoke test remains describe the legacy implementation and are no longer the completion contract. The new scope includes a pinned configured web model, parallel safety/extraction with decoys, separate final screening, checked multi-format sources, background lifecycle, mandatory whole-harness protection and durable provenance, hostname incidents and user-only resolution. Existing raw/quarantine-off paths and direct search snippets do not satisfy it. This is specification work only: no implementation plan, runtime changes or live trials have been authorized or performed; the epic remains in progress and shared security/routing dependencies require reconciliation before implementation.

## Acceptance Criteria

- Preserve the shipped cozy-tools web extraction, published dependency and helper integration as the legacy baseline, not proof of the redesigned feature.
- Deliver the confirmed protected asynchronous web research contract in specs/protected-web-research.md: explicit user-selected capable model, parallel quarantined safety/extraction, final candidate screening, checked evidence and immutable reusable sources.
- Support public static pages, PDF, isolated JavaScript rendering and visual/OCR research; declare unavailable safe modes instead of silently weakening isolation.
- Prevent whole-harness bypass and preserve material restrictions across turns, resume, forks, compaction and derivatives; restricted protected mode may disable unmediated tools.
- Provide user-wide canonical hostname blocking for source-attributed decoy calls, dependency revocation, human-only incident investigation and user-controlled recheck/unblock without unchecked raw delivery.
- Provide bounded background jobs with origin-bound delivery, explicit resume, protected scoped caches and shared account admission favoring interactive work.
- Meet the agreed finite-corpus acceptance: zero successful release/action/egress bypasses and at most 5% benign-task false stops; report quality, coverage, latency and usage without universal safety claims.

## Verification Plan

1. Use the approved public web/session and lifecycle seams and real process/filesystem/egress boundaries from specs/protected-web-research.md; fake model/network fixtures establish deterministic harness behavior.
2. Exercise safe multi-format acquisition, parallel-release races, decoys, final screening, source reuse, revocation, grants, recipient control and bypass prevention on each supported platform.
3. Run separately authorized finite attack and benign-task evaluations with actual release/action/egress outcomes, false-stop accounting, quality, latency and usage measurements.
4. Keep implementation gates scoped to changed files/packages; specification publication alone does not close this epic.

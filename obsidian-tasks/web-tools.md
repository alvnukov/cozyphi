---
id: web-tools
title: Веб-тулы (search/fetch/find/read) из mcp-ai-helper через cozy-tools в козю
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
    - cozy-tools получает семейство web (webfetch, websearch, WebPolicy как данные в config-шве) с тестами, перенесёнными из mcp-ai-helper и дополненными; README/RELEASE обновлены (web больше не host-only).
    - mcp-ai-helper импортирует web из cozy-tools и удаляет свои internal/webfetch, internal/websearch; поведение MCP-тулы web не меняется.
    - cozyphi получает нативную тулу `web` с действиями search/fetch/find/read (internal/tools/webtool), permission.ActionWeb со своим ключом политики, дефолт ask с деталью URL/запрос; секция web в ~/.cozyphi/config.yaml → WebPolicy, кеш ~/.cozyphi/web, ключ Google только из env.
    - Тексты read/find и сниппеты search приходят модели в явной рамке «untrusted web content», описание тулы говорит то же; fetch отдаёт только метаданные; лимиты read 4000/20000 и find как в helper.
    - Дети получают web по потолку роли; sub-agent ceiling документирован; doc/web.md, project-layout, AGENTS.md (инвариант про web как untrusted egress), CHANGELOG.
verification_plan:
    - cozy-tools go test ./... (это библиотека, там гейт репо-wide по их правилам); в helper и козе — только затронутые пакеты.
    - Тесты SSRF (private IP на URL и на dial, localhost, схемы, редиректы), лимитов, кеша, рамки untrusted, гейта (ask по умолчанию, deny в политике).
    - Ручная проверка в сессии: web search → fetch → find → read на публичной странице.
created_at: "2026-09-07T11:00:00Z"
updated_at: "2026-09-07T00:17:12Z"
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

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
updated_at: "2026-09-07T11:00:00Z"
---

## Body

**Откуда.** Запрос пользователя 2026-09-07: в helper есть веб-тулы, в козе нет; взять через cozy-tools, сделав безопасными для модели.

**Что уже есть в helper (internal/mcp/web_tools.go, internal/webfetch, internal/websearch, ~1160 строк, только stdlib).** Одна MCP-тула `web` с action search/fetch/find/read. Fetch кладёт source.bin + normalized.txt + metadata.json в кеш по doc_id (sha256 URL+тела), модели отдаёт только метаданные. Read — ограниченный фрагмент, Find — сниппеты с офсетами. Search: duckduckgo_html (регексы по HTML) и google_cse (ключ из env, редакция ключа в ошибках). WebPolicy: enabled, cache_dir, max_source_bytes, timeout, max_redirects, allowed/denied hosts, схемы, content types, UA, провайдер. Защита от SSRF на двух уровнях: validateURL (схема, localhost, непубличный IP, deny/allow hosts) и dial guard по резолвнутому адресу (обходится только при явном allowed_hosts).

**Чего не хватает для «безопасно для модели».** Рамки untrusted вокруг текста страницы (главный риск — prompt injection со страницы); интеграции с permission gate козы; своего кеш-пути и конфига; документации потолка для детей.

**Шаги.** 1 cozy-tools: пакеты webfetch/websearch + WebPolicy (агент в репо cozy-tools). 2 параллельно после 1: helper переезжает на cozy-tools; коза получает internal/tools/webtool + ActionWeb + config + рамка + доки.

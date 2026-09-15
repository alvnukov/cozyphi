---
id: utf8-safe-truncate-helper
title: Обрезка строк режет UTF-8 посередине руны — два общих helper'а вместо двенадцати копий
status: done
priority: high
model_level: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - bug
    - utf8
    - dry
    - permissions
branch: bug/utf8-safe-truncate-helper
worktree_path: .worktrees/utf8-safe-truncate-helper
acceptance_criteria:
    - internal/util/text.go держит rune-safe обрезку с маркером и байтовую обрезку с откатом на границу руны; обе документированы
    - 'тест-регрессия: обрезка кириллицы на каждой длине из диапазона даёт валидный UTF-8 (utf8.ValidString) — падает на старой байтовой реализации'
    - 'двенадцать локальных helper''ов удалены: permission/gate.go, mcp/util.go, cmd/run.go, memory/recall.go, tools/memorytool/memory.go, watch/watch.go, provider/quota.go, provider/oauth.go, tools/agenttool/agent.go, session/compaction/compact.go, tui/ctxpane/pane.go, tools/lsptool/render.go'
    - 'ещё четыре копии жили прямо в телах функций и тоже переведены на общий helper: session/load.go, session/compaction/micro.go, tools/harnesstool/harness.go, mcp/compact.go (FormatCallResult вдобавок называл байты символами)'
    - текст запроса на одобрение bash и watch больше не содержит битых рун
    - 'каждая точка вызова получает честную единицу: руны для отображаемого текста, байты для бюджета на недоверенных данных'
    - маркер "\n…(truncated)" у summary sub-agent'а сохранён (закреплён тестом wait_legacy_test.go)
    - строка в CHANGELOG под [Unreleased]
    - подписанный Conventional Commit на ветке задачи в .worktrees/
verification_plan:
    - 'go test по изменённым пакетам: util, permission, mcp, memory, tools/memorytool, tools/agenttool, tools/harnesstool, tools/lsptool, watch, provider, session, session/compaction, tui/ctxpane, cmd'
    - один golangci-lint run по изменённым пакетам
    - 'grep: в репозитории не осталось среза вида s[:n] + "…" без отката на границу руны'
    - 'живой прогон: bash-команда с кириллическим путём длиннее лимита — промпт одобрения показывает целые символы'
created_at: "2026-09-15T23:12:05.530913Z"
updated_at: "2026-09-15T23:28:39.932622Z"
---

## Body

**Дефект.** `truncate(s, n)` вида `s[:n] + "…"` режет строку по байтам. На не-ASCII тексте срез попадает в середину руны, и наружу уходит битый UTF-8. Проверено перебором лимитов 0..59 на кириллическом пути: 19 срезов из 60 пришлись в середину руны.

**Где это видит пользователь.** internal/permission/gate.go:328 — текст запроса на одобрение bash-команды: `"bash requires approval: " + truncate(cmd, 120)`. Команда с кириллицей длиннее 120 байт даёт в промпте `.../ката<U+FFFD>`. Рядом gate.go:298 (watch), gate.go:257 (путь), extract.go:313, taint.go:83.

**Масштаб копипасты.** Один и тот же helper продублирован дословно трижды: internal/permission/gate.go:504, internal/mcp/util.go:5, cmd/run.go:790. Ещё три копии — rune-корректные, тоже дословные друг другу: internal/memory/recall.go:268, internal/tools/memorytool/memory.go:334, internal/watch/watch.go:496. Итого шесть реализаций одной функции, из них три написаны неверно.

**Тот же дефект под другими именами:** internal/provider/quota.go:611 `truncateText`, internal/provider/oauth.go:320 `truncateOAuthError`, internal/tools/agenttool/agent.go:458 `truncateBytes`. Ещё две копии нашлись при проверке уже по ходу работы: internal/tools/harnesstool/harness.go:245 `safeEcho` и internal/mcp/compact.go:118 `FormatCallResult` — срез по байтам вшит прямо в тело, причём второй ещё и подписывал результат «chars total», хотя считал байты. Плюс rune-корректные, но всё те же дословные копии: internal/session/compaction/compact.go:330 `truncateForSummary`, internal/tui/ctxpane/pane.go:703 `truncateRunes`, internal/session/load.go:128 `truncatePreview`. И internal/tools/lsptool/render.go:156 `bound` — с комментарием, утверждающим ровно обратное: «never leaving a partial surrogate problem since it works on raw bytes of already-valid UTF-8 text». Двенадцать helper'ов и четыре вшитых среза — одна работа.

**Единица длины — не косметика.** Для отображаемого текста (команда, путь, описание) естественны руны. Для бюджета размера на недоверенных байтах (сырое тело HTTP-ответа в internal/mcp, ошибка OAuth-сервера, потолок summary у sub-agent'а) честны байты — но срез обязан ложиться на границу руны. Поэтому helper'ов два, а не один; в репозитории это уже открыли независимо — internal/plangate/projection.go:499 и internal/session/compaction/micro.go:269 режут по байтам с откатом на границу руны.

**Дом.** internal/util/text.go — лист без единой внутренней зависимости, уже держит `NormalizeLF`.

**Осознанно вне объёма.** internal/plangate/projection.go `truncateBytes` (иной контракт: правит на месте и возвращает «резал ли») и `truncateRunes` (без маркера, маркер лепит вызывающий, внутри деликатной арифметики бюджета проекции); internal/components/input/line.go `truncateRunes` (примитив раскладки каретки, маркер не нужен); internal/tools/agenttool `truncateRunes` (ярлык без маркера); internal/components/layout `TruncateToWidth` (режет по ширине ячеек терминала, а не по рунам — другая задача).

## Acceptance Criteria

- internal/util/text.go держит rune-safe обрезку с маркером и байтовую обрезку с откатом на границу руны; обе документированы
- тест-регрессия: обрезка кириллицы на каждой длине из диапазона даёт валидный UTF-8 (utf8.ValidString) — падает на старой байтовой реализации
- двенадцать локальных helper'ов удалены: permission/gate.go, mcp/util.go, cmd/run.go, memory/recall.go, tools/memorytool/memory.go, watch/watch.go, provider/quota.go, provider/oauth.go, tools/agenttool/agent.go, session/compaction/compact.go, tui/ctxpane/pane.go, tools/lsptool/render.go
- ещё четыре копии жили прямо в телах функций и тоже переведены на общий helper: session/load.go, session/compaction/micro.go, tools/harnesstool/harness.go, mcp/compact.go (FormatCallResult вдобавок называл байты символами)
- текст запроса на одобрение bash и watch больше не содержит битых рун
- каждая точка вызова получает честную единицу: руны для отображаемого текста, байты для бюджета на недоверенных данных
- маркер "\n…(truncated)" у summary sub-agent'а сохранён (закреплён тестом wait_legacy_test.go)
- строка в CHANGELOG под [Unreleased]
- подписанный Conventional Commit на ветке задачи в .worktrees/

## Verification Plan

1. go test по изменённым пакетам: util, permission, mcp, memory, tools/memorytool, tools/agenttool, tools/harnesstool, tools/lsptool, watch, provider, session, session/compaction, tui/ctxpane, cmd
2. один golangci-lint run по изменённым пакетам
3. grep: в репозитории не осталось среза вида s[:n] + "…" без отката на границу руны
4. живой прогон: bash-команда с кириллическим путём длиннее лимита — промпт одобрения показывает целые символы

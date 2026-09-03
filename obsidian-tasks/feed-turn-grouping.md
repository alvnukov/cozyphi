---
id: feed-turn-grouping
title: Turn grouping and auto-condensation of finished turns
status: done
tags:
    - ui
    - ux-standard
    - feed
created_at: "2026-09-01T17:22:34.000000Z"
updated_at: "2026-09-01T18:14:55.000000Z"
---

## Body

**Проблема.** В ленте нет понятия «ход» (user-сообщение → финальный текст ассистента): после длинной автономной сессии это сотни tool-строк одинакового веса, важное не всплывает, скролл бесконечный. Другие харнесы (Codex CLI, Claude Code) сворачивают завершённую работу в резюме.

**Что сделать.** (1) Ввести ход в проекцию ленты (`session.Project` / transcript mapper). (2) Завершённые ходы старше последних 1–2 автоматически конденсируются в резюме-гуттер «▸ worked 42s · 7 tools · pane.go, mapper.go · 12k tok», разворачиваемый по Enter/клику. (3) Глобальный verbose-переключатель (все тела открыты / стандартная свёртка) — одной клавишей, в таблице keys. (4) Прыжки по ходам с клавиатуры (предыдущий/следующий user-блок). Ошибки и отклонённые действия не конденсируются — остаются видимыми и в свёрнутом ходе. Строится на статах из [[feed-semantic-tool-rows]].

**Критерий.** Старые ходы автоматически сжаты до строки-резюме с длительностью, числом тулов и тронутыми файлами; Enter разворачивает ход целиком; verbose-режим включается одной клавишей и переживает перерисовку; ошибка внутри свёрнутого хода видна без разворачивания.

**Результат.** Сделано (commit 8136e7d, merge в main 958df15). (1) Ход введён на уровне mapper, не проекции: `groupTurns` над `session.Project` — `session.Project` остаётся чистым сплющиванием, а `syncTail` остаётся валидным, потому что хвостовой ход никогда не группируется. Границы хода — отправленные user-строки (queued не граница, очищается редьюсером при отправке); ходы старше последних двух (`keepFullTurns=2`) конденсируются: user-строка и финальный ответ остаются, рабочие строки (thinking, тулы, промежуточный текст) складываются за резюме-строку `block.TurnSummaryBlock` «▸ worked 42s · 7 tools · pane.go, mapper.go» (Muted, длительность из `turnDurations` по TurnDuration ассистентских раундов, файлы — base-имена из edit/write Detail, клип >3 файлов «+N», фолбэк «N steps»). (2) Разворот кликом/Enter: OnToggle пишет expand-state и дёргает `onRegroup` → полный resync, скрытые строки возвращаются, резюме остаётся ручкой складывания с ▾; TurnSummaryBlock исключён из snapshot-цикла expand-состояний (как DiffBlock) — только явный toggle записывается. (3) Verbose — `CmdVerbose` («transcript-verbose», Ctrl+E) в таблице keys: editor → `TranscriptPane.ToggleVerbose` → `Mapper.SetVerbose`, тост называет режим; состояние живёт в mapper и переживает любые перерисовки. (4) Прыжки по ходам — Shift+PgUp/PgDn в `HandlePageKey` (composer уже пробрасывает PgUp/PgDn с любыми модификаторами): `JumpTurn` ищет предыдущий/следующий не-queued UserBlock от верха вьюпорта и скроллит через новые `MessageList.TopEntryIndex`/`ScrollToEntry`; за последним ходом — прищёлкивание к хвосту, перед первым — верх. Ошибки: `keepVisible` не даёт свёрнутому ходу спрятать error/rejected-тул, queued-подсказку и compaction-маркер; резюме считает «N failed» (Destructive). Тесты: mapper_turns_test.go (5: конденсация+статы, видимость упавшего тула, toggle раскрыть/сложить, verbose, прыжки Shift+PgUp/PgDn), turn_summary_block_test.go (4: лейбл, фолбэк, клип файлов, toggle/стрелка). Каталог keys дополнен (Shift+PgUp/PgDn, Ctrl+E в Transcript-группе), DESIGN.md «The transcript feed» расширен абзацем о конденсации, запись в CHANGELOG. Гейт rc=0, happ-диагностика чистая, main `make test` rc=0.

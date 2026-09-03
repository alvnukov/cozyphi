---
id: claude-consult-tool
title: 'Тула claude: точечные консультации через Claude Code headless'
status: todo
priority: high
model_level: high
task_type: feature
tags:
    - claude
    - tools
    - epic
    - economy
acceptance_criteria:
    - Модель cozyphi одним вызовом claude получает структурированный ответ в любом из шести режимов; вызов проходит через job.Manager и виден в TUI как job с прогрессом и ценой.
    - 'Бриф собирает харнесс: файлы инлайнятся в бюджете, план и diff прикладываются автоматически для review_plan/review_diff; ответ парсится по JSON-схеме режима.'
    - Follow-up идёт в тред (--resume); лимит вызовов за ход, дедуп и ledger usage/cost работают; implement стартует только после Ask и по умолчанию в worktree.
    - Sub-agents тулу не имеют; permission gate не обходится; секреты не попадают в транскрипт и логи; отсутствие бинаря не ломает старт.
    - doc/claude.md, README, project-layout, AGENTS.md и CHANGELOG описывают тулу; все дети эпика done с зелёным gate.
verification_plan:
    - 'Пройти по детям: у каждого status done и зелёный make fmt-check lint test на финальном коммите.'
    - 'Ручной e2e в TUI: review_plan на реальном плане, review_diff на реальном diff, implement в worktree с Ask, thread follow-up — с проверкой, что в результате есть usage/cost и что бриф уложился в бюджет.'
    - 'Проверить economy-инварианты: повторный вызов с тем же брифом отдаёт cached, четвёртый вызов за ход отклоняется, sub-agent не видит тулу.'
created_at: "2026-09-02T05:13:21.747712Z"
updated_at: "2026-09-02T05:13:21.747712Z"
---

## Body

**Цель.** Дать модели cozyphi (как правило, дешёвой) и пользователю способ привлекать Claude там, где он окупается: архитектурные решения, проверка и правка плана до approve, ревью diff критичного кода, диагноз застревания, исследование, которое дешёвая модель не вытянула, и написание критичного кода в изоляции. Claude — консультант, а не ещё один sub-agent: он получает собранный харнессом бриф, а не кодовую базу, и отвечает по JSON-схеме.

**Принцип экономии.** Токены Claude идут на суждение, а не на чтение репозитория. Рычаги: (1) бриф с инлайном выдержек вместо Read-раундов; (2) треды через --resume вместо повторного брифа (prompt cache); (3) урезанный контекст самого Claude Code: --strict-mcp-config, --disable-slash-commands, --tools только нужные, --setting-sources по режиму; (4) --json-schema — короткий парсируемый ответ; (5) модель и effort по режиму; (6) тормоза: лимит вызовов за ход, дедуп по хэшу брифа, implement только через Ask; (7) видимая цена: usage/cost/turns из stream-json в ledger и в строке результата. --bare не используется: он запрещает OAuth, а экономятся в первую очередь лимиты подписки.

**Архитектура (зафиксировано).** Пакет internal/claude — глубокий модуль: конфиг ~/.cozyphi/claude.json по образцу internal/lsp/config.go (owner-controlled, fail-closed), резолв бинаря ~/.cozyphi/bin → PATH, запуск claude -p --output-format stream-json через internal/proc, разбор потока, пресеты режимов, сборка брифа, типизированный разбор ответа. Второй адаптер шва job.Runner (сейчас у него один — EngineRunner): ClaudeRunner за тем же job.Manager, так что agent_wait/agent_list/agent_cancel, jobs-каталог, прогресс в TUI, таймауты и recovery достаются бесплатно; Meta получает Backend. Одна модель-facing тула claude с mode ∈ {architect, review_plan, review_diff, diagnose, research, implement}: блокируется до timeout_sec, иначе отдаёт job_id. Permission: ActionClaude — read-only режимы Allow, implement Ask с именем каталога. Sub-agents и дети тулу не получают; headless cozyphi run получает. Env процесса sanitized; вывод через redact.

**Куда Claude не зовём (в описание тулы).** Компакция, commit-сообщения, рутинные правки, всё, что отвечают grep/lsp, повторный бриф вместо thread, exploration без вопроса.

**Порядок работ.** runner → modes/brief → job backend → tool+gate (шов, на котором всё держится) → attachments → economy → implement isolation → TUI → stuck detector → docs; онбординг/кураторство памяти и plan action — фаза 2 (priority low). Зависимости названы в теле каждой задачи.

**Уровни моделей.** По умолчанию medium: дизайн зафиксирован в телах задач, образцы в репозитории (lsp config, proc, watch, agenttool). high — только там, где решение шва или эвристики определяет качество всего эпика: тула+gate+описание, stuck detector (executor loop), plan action (approval/transitions). Каждая задача — один tracer bullet за одним публичным швом; закрытие через mcp-ai-helper workflow с гейтом make fmt-check lint test.

## Acceptance Criteria

- Модель cozyphi одним вызовом claude получает структурированный ответ в любом из шести режимов; вызов проходит через job.Manager и виден в TUI как job с прогрессом и ценой.
- Бриф собирает харнесс: файлы инлайнятся в бюджете, план и diff прикладываются автоматически для review_plan/review_diff; ответ парсится по JSON-схеме режима.
- Follow-up идёт в тред (--resume); лимит вызовов за ход, дедуп и ledger usage/cost работают; implement стартует только после Ask и по умолчанию в worktree.
- Sub-agents тулу не имеют; permission gate не обходится; секреты не попадают в транскрипт и логи; отсутствие бинаря не ломает старт.
- doc/claude.md, README, project-layout, AGENTS.md и CHANGELOG описывают тулу; все дети эпика done с зелёным gate.

## Verification Plan

1. Пройти по детям: у каждого status done и зелёный make fmt-check lint test на финальном коммите.
2. Ручной e2e в TUI: review_plan на реальном плане, review_diff на реальном diff, implement в worktree с Ask, thread follow-up — с проверкой, что в результате есть usage/cost и что бриф уложился в бюджет.
3. Проверить economy-инварианты: повторный вызов с тем же брифом отдаёт cached, четвёртый вызов за ход отклоняется, sub-agent не видит тулу.

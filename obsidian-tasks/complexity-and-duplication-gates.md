---
id: complexity-and-duplication-gates
title: Сложность, дублирование и границы импортов — под гейтом-храповиком от текущего худшего
status: done
priority: high
model_level: high
task_type: chore
parent_id: cozyphi-enterprise-code-review
tags:
    - lint
    - architecture
    - ci
    - complexity
branch: chore/complexity-and-duplication-gates
worktree_path: .worktrees/complexity-and-duplication-gates
acceptance_criteria:
    - в .golangci.yml включены cyclop, gocognit, funlen, nestif, dupl, interfacebloat
    - 'каждый порог равен текущему худшему показателю репозитория и подписан функцией, которая его задала; это храповик — порог только опускают'
    - 'internal/arch проверяет границы слоёв по реальному графу импортов, а не по абзацу в документации'
    - 'scripts/lint-ceilings.sh пересчитывает весь набор потолков и печатает, кто задал каждый; доступен как make lint-ceilings'
    - make lint зелёный на всём репозитории с включёнными линтерами
    - 'ни один порог не куплен исключением без письменной причины: три исключения dupl объяснены в конфиге'
    - строка в CHANGELOG под [Unreleased]
    - подписанный Conventional Commit на ветке задачи в .worktrees/
verification_plan:
    - go test ./internal/arch/
    - 'make lint по всему репозиторию (здесь это и есть предмет задачи, а не разовый прогон по изменённым пакетам)'
    - 'scripts/lint-ceilings.sh воспроизводит ровно те числа, что закреплены в .golangci.yml и boundaries_test.go'
    - 'мутационная проверка правил: снижение fanOutCeiling и снятие записанного исключения роняют тест с внятным сообщением'
    - golangci-lint config verify
created_at: "2026-09-15T23:44:22.891738Z"
updated_at: "2026-09-15T23:44:22.891738Z"
---

## Body

**Находка (ревью 2026-09-16).** Метрики сложности в репозитории никто не держит: `internal/agent.(*Engine).Loop` — когнитивная сложность 155, `internal/tui/commands.registerBuiltinCommands` — 486 строк, `commands.Host` — 40 методов, `internal/tui/sidebar.(*Sidebar).Handle` — вложенность 40. Архитектурные границы («util — лист», «TUI никто не импортирует», «нижние слои не тянутся вверх») существовали только как договорённость: ни одна из них не проверялась, и проверить их постфактум было нечем.

**Решение — храповик, а не цель.** Пороги выставлены по текущему худшему значению: не «так можно писать», а «хуже стать нельзя». Каждое число подписано функцией, которая его задала, и опускается, как только эту функцию разобрали. Поднимать порог, чтобы билд прошёл, запрещено текстом рядом с числом.

**Что закреплено.** cyclop: max-complexity 73 (`composer.(*ComposerPane).Handle` и `lsp.TestFakeLSP`), package-average 9 (худший — тесты `internal/tui/sessions`, 8.7). funlen: 486 строк и 199 инструкций. gocognit: 155. nestif: 41 — на единицу выше `sidebar.Handle`, потому что nestif срабатывает **на** пороге, а не выше него. interfacebloat: 40 (`commands.Host`). dupl: 150 токенов — свой же дефолт; два оставшихся семейства клонов названы в исключениях, а не куплены поднятием порога.

**Три исключения dupl и их причина.** Повтор в тесте обычно и есть смысл теста — два теста на общей фикстуре падают вместе, и setup читателю нужен перед глазами. `internal/components/theme.go` — таблица: каждая тема называет те же поля своими цветами, сворачивание в цикл спрячет значения, ради которых файл существует. `internal/components/block/*_block.go` действительно почти копии — их разбирает задача dedupe-draw-primitives-and-load-facts, а не поднятый порог.

**Границы как тест.** `internal/arch` — пакет без собственного кода: граница есть факт обо всех остальных пакетах, поэтому не принадлежит ни одному. Пять тестов поверх `go list -json` (не `x/tools/go/packages` — репозиторий считает зависимости, а голый `go list` даёт то же самое): util остаётся листом; терминальный слой — сток, в него импортируют только `cmd`; фреймворк `xui` не протекает за пределы терминального слоя; восемь правил «нижний слой не тянется вверх» (llm, session, permission, tools, provider, diag, memory, mcp против agent/session/tools); fan-out пакета не выше 47 (`internal/tui/sessions`).

**Записанное исключение — одно.** `internal/project` импортирует `internal/tui/keys`, потому что валидирует имена биндов из конфига, а имена принадлежат UI, который их связывает. Исключение лежит в таблице с объяснением, как его снять (опустить таблицу имён ниже UI), а не в молчаливом `if`.

**Операбельность.** `scripts/lint-ceilings.sh` (`make lint-ceilings`) меряет весь набор заново и печатает `ceiling / value / set by`. Без него храповик — археология: через полгода никто не знает, какая функция держит 486.

**Найдено по ходу.** golangci-lint по умолчанию держит `issues.uniq-by-line: true` и оставляет одну находку на строку по всем линтерам сразу. Все эти линтеры указывают на строку объявления функции, поэтому первое измерение врало: `Engine.Loop` (255 строк) не попадал в отчёт funlen, его строку уже занял gocognit. Плюс funlen сообщает либо про строки, либо про инструкции, но не про обе сразу — отсюда два прохода в скрипте.

**Осознанно вне объёма.** Сам разбор `Engine.Loop`, `registerBuiltinCommands`, `Host` и блочных рендереров — отдельные задачи ревью (controller-god-object-decomposition, split-commands-host-interface, dedupe-draw-primitives-and-load-facts). Эта задача ставит гейт, который не даст им отрасти обратно.

## Acceptance Criteria

- в .golangci.yml включены cyclop, gocognit, funlen, nestif, dupl, interfacebloat
- каждый порог равен текущему худшему показателю репозитория и подписан функцией, которая его задала; это храповик — порог только опускают
- internal/arch проверяет границы слоёв по реальному графу импортов, а не по абзацу в документации
- scripts/lint-ceilings.sh пересчитывает весь набор потолков и печатает, кто задал каждый; доступен как make lint-ceilings
- make lint зелёный на всём репозитории с включёнными линтерами
- ни один порог не куплен исключением без письменной причины: три исключения dupl объяснены в конфиге
- строка в CHANGELOG под [Unreleased]
- подписанный Conventional Commit на ветке задачи в .worktrees/

## Verification Plan

1. go test ./internal/arch/
2. make lint по всему репозиторию (здесь это и есть предмет задачи, а не разовый прогон по изменённым пакетам)
3. scripts/lint-ceilings.sh воспроизводит ровно те числа, что закреплены в .golangci.yml и boundaries_test.go
4. мутационная проверка правил: снижение fanOutCeiling и снятие записанного исключения роняют тест с внятным сообщением
5. golangci-lint config verify

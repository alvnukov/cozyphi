---
id: voice-space-repeat-event-type
title: Различать автоповтор клавиши по kitty event_type вместо тайминга
status: done
priority: medium
model_level: high
task_type: bug
tags:
    - voice
    - xui
    - input
    - tui
acceptance_criteria:
    - '`input.KeyEvent` получает поле `Repeat bool`: true, когда терминал сообщил kitty event_type 2 (автоповтор ОС). У такого события `Press` остаётся true — `Repeat` уточняет нажатие, а не заменяет его.'
    - '`parseModField` различает все три типа события (1 нажатие, 2 автоповтор, 3 отпускание) и прокидывает результат во все три места разбора: `~`-клавиши (parser.go:323), курсорные клавиши (parser.go:448) и `parseCSIu` (parser.go:514) — Space идёт через последний, поэтому флаг обязан доходить и туда.'
    - '`ComposerPane.pressSpace` не переключает микрофон на событии с `Repeat: true` — независимо от таймингов; окно только сдвигается, как сейчас.'
    - Скользящее окно (`tapRepeatWindow` / `holdRepeatWindow`) сохранено как запасной путь для терминалов без kitty-протокола, где `Repeat` не приходит никогда. Ни одна из констант не удалена.
    - 'Событие с `Repeat: true` включает `releasesSeen` наравне с настоящим отпусканием: флаг 2 kitty-протокола («report event types») выдаёт повторы и отпускания вместе, поэтому повтор — такое же честное доказательство, что отпускания будут.'
    - 'Тесты в `xui/input`: разбор event_type 1/2/3 в форме CSI u и в курсорной форме — press/repeat выставлены верно.'
    - 'Тест в `internal/tui/composer`: серия автоповторов не переключает микрофон даже когда пауза между ними больше `tapRepeatWindow` (медленная настройка автоповтора ОС — сегодня это ложное второе нажатие).'
    - '`xui/PATCH_NOTES.md` дополнен новым пунктом списка локальных расхождений с upstream v0.1.3.'
    - Строка добавлена наверх `## [Unreleased]` в CHANGELOG.md.
verification_plan:
    - go build ./...
    - go test ./xui/input/... ./internal/tui/composer/...
    - go test ./...
    - golangci-lint run один раз, только по изменённым пакетам, непосредственно перед коммитом
created_at: "2026-09-04T08:39:57.196648Z"
updated_at: "2026-09-04T08:52:44.189957Z"
---

## Body

**Зачем.** Фикс `voice-space-hold-repeat` научил composer отличать удержание Space от повторных нажатий по таймингу: скользящее окно `tapRepeatWindow` (600 мс) / `holdRepeatWindow` (2 с) в `internal/tui/composer/voice.go`. Но терминал и так говорит нам правду напрямую — мы её выбрасываем на парсере.

**Что происходит сейчас.** `xui` включает kitty keyboard protocol с флагами `>7u` (`xui/render/ctlseqs.go:35`), где бит 2 — «report event types». Терминал присылает три типа события: 1 — нажатие, 2 — автоповтор ОС, 3 — отпускание. `parseModField` (`xui/input/parser.go:396`) проверяет только `if et == 3 { press = false }`, а 1 и 2 схлопывает в один `press = true`. В `input.KeyEvent` (`xui/input/event.go:11`) есть только `Press bool`, поля `Repeat` нет. Поэтому composer не может отличить автоповтор от настоящего нового нажатия и вынужден угадывать по времени.

**Где это реально ломается.** Комментарий у `tapRepeatWindow` честно пишет, что 600 мс покрывают не все настройки автоповтора macOS. При медленной настройке первый автоповтор приходит позже окна, `pressSpace` считает его новым нажатием и микрофон делает лишний переворот — тот самый баг, который фиксили, только на длинном хвосте настроек.

**Что делаем.** Проносим event_type до `KeyEvent.Repeat` и в `pressSpace` отбрасываем повторы по флагу, а не по часам. Окно остаётся ровно для терминалов без протокола (Terminal.app, часть tmux-конфигов), где повторы неотличимы в принципе.

**Границы.** Меняем вендоренный `xui` — он лежит в этом же репозитории (`replace github.com/pulseaiclub/xui => ./xui`), правки идут в `xui/input/event.go` и `xui/input/parser.go` и обязаны быть записаны в `xui/PATCH_NOTES.md` как очередной пункт локальных расхождений. Поле добавляется, ничего не переименовывается: существующие потребители `KeyEvent` не трогаем, `Press` сохраняет смысл. Никаких изменений в звуковом слое `internal/voice` — задача целиком про ввод.

**Файлы.** `xui/input/event.go`, `xui/input/parser.go`, `xui/input/parser_test.go`, `xui/PATCH_NOTES.md`, `internal/tui/composer/voice.go`, `internal/tui/composer/pane.go`, `internal/tui/composer/voice_test.go`, `CHANGELOG.md`.

## Acceptance Criteria

- `input.KeyEvent` получает поле `Repeat bool`: true, когда терминал сообщил kitty event_type 2 (автоповтор ОС). У такого события `Press` остаётся true — `Repeat` уточняет нажатие, а не заменяет его.
- `parseModField` различает все три типа события (1 нажатие, 2 автоповтор, 3 отпускание) и прокидывает результат во все три места разбора: `~`-клавиши (parser.go:323), курсорные клавиши (parser.go:448) и `parseCSIu` (parser.go:514) — Space идёт через последний, поэтому флаг обязан доходить и туда.
- `ComposerPane.pressSpace` не переключает микрофон на событии с `Repeat: true` — независимо от таймингов; окно только сдвигается, как сейчас.
- Скользящее окно (`tapRepeatWindow` / `holdRepeatWindow`) сохранено как запасной путь для терминалов без kitty-протокола, где `Repeat` не приходит никогда. Ни одна из констант не удалена.
- Событие с `Repeat: true` включает `releasesSeen` наравне с настоящим отпусканием: флаг 2 kitty-протокола («report event types») выдаёт повторы и отпускания вместе, поэтому повтор — такое же честное доказательство, что отпускания будут.
- Тесты в `xui/input`: разбор event_type 1/2/3 в форме CSI u и в курсорной форме — press/repeat выставлены верно.
- Тест в `internal/tui/composer`: серия автоповторов не переключает микрофон даже когда пауза между ними больше `tapRepeatWindow` (медленная настройка автоповтора ОС — сегодня это ложное второе нажатие).
- `xui/PATCH_NOTES.md` дополнен новым пунктом списка локальных расхождений с upstream v0.1.3.
- Строка добавлена наверх `## [Unreleased]` в CHANGELOG.md.

## Verification Plan

1. go build ./...
2. go test ./xui/input/... ./internal/tui/composer/...
3. go test ./...
4. golangci-lint run один раз, только по изменённым пакетам, непосредственно перед коммитом

---
id: voice-resident-whisper-server
title: 'voice: резидентный whisper-server STT-бэкенд'
status: done
priority: high
task_type: feature
tags:
    - voice
    - performance
branch: feature/voice-resident-whisper-server
worktree_path: .worktrees/voice-resident-whisper-server
acceptance_criteria:
    - 'voice.stt.backend понимает server: cozyphi запускает whisper-server с найденной моделью на свободном порту 127.0.0.1'
    - Ленивый старт на первом сегменте, переживает паузы внутри диалог-режима, убивается на End/Discard/Close после дренажа очереди
    - Готовность (загрузка модели) ожидается с bounded timeout; ошибка старта — одна строка с подсказкой
    - auto резолвится в порядке server → command → http; /voice status называет whisper-server и модель
    - Командная строка сервера конфигурируется с плейсхолдерами {model} {port}; дефолт проверен живым запуском на этой машине
    - make fmt-check lint test зелёные в worktree задачи; после выхода нет осиротевшего whisper-server
verification_plan:
    - 'go test ./internal/voice/... в worktree: резолв бэкенда, lifecycle (Close зовётся на End/Discard), ready-wait таймаут, запрос на /inference'
    - 'Живой smoke: whisper-server поднят cozyphi, второй сегмент не платит за загрузку модели; ps после выхода чист'
    - make fmt-check lint test в worktree
created_at: "2026-09-03T22:50:50.630495Z"
updated_at: "2026-09-04T07:53:48.643758Z"
---

## Body

Проблема: дефолтный command-бэкенд запускает whisper-cli на каждый
сегмент, и ggml-модель перечитывается с диска заново — с ggml-small это
секунды латентности на каждую фразу в диалоговом режиме.

Решение: третий бэкенд `server`. cozyphi управляет резидентным
whisper-server: ленивый старт на первом сегменте режима, свободный порт
через net.Listen("tcp","127.0.0.1:0"), ожидание готовности с таймаутом,
HTTP-запросы сегментов (обобщить HTTPTranscriber до настраиваемого пути
эндпоинта — whisper.cpp слушает /inference, не /audio/transcriptions),
убийство процесса при завершении режима после дренажа очереди воркера.
Командная строка конфигурируется с плейсхолдерами {model} {port}; в
auto порядок server → command → http.

Контракт whisper-server (флаги, эндпоинты, поля multipart) плавает между
версиями — пиновать живым запуском на этой машине в первом шаге.

Ожидаемые правки: internal/voice/{config,stt_server,session}.go,
internal/voice/*_test.go, doc/voice.md, CHANGELOG.md. Соседняя задача
voice-model-install трогает тот же резолвер — координировать при мерже.

**Started (2026-09-04).** Принята в работу: план одобрен (7 шагов: worktree → контракт whisper-server → config → stt_server.go → session lifecycle → docs → gates).

**Note (2026-09-04).** Контракт whisper-server (homebrew, ggml 0.20.0, whisper.cpp ~v1.7.x, Apple M2): старт `whisper-server -m <model> --host 127.0.0.1 --port <N>`; флаги --inference-path (дефолт /inference), -l/--language ('auto' валиден), --prompt. Готовность: HTTP-листенер поднимается ПОСЛЕ загрузки модели — poll GET / до 200; с ggml-small ~1.2 с (Metal). Запрос: POST /inference, multipart поля file + response_format=json + language (omit при auto) + prompt — все приняты; ответ {"text": "..."} с \n внутри (NormalizeTranscript уже схлопывает). Латентность: 11-с jfk.wav 0.4 с против ~3.3 с у whisper-cli на сегмент. Models на машине: ~/.cozyphi/models/{ggml-small,ggml-large-v3-turbo}.bin.

**Done (2026-09-04).** Ветка `feature/voice-resident-whisper-server`, коммит `1725cc7` (10 файлов, +539/−45):

- `internal/voice/stt_server.go` — ServerTranscriber: свободный порт (net.ListenConfig, :0), spawn через proc.Start, waitReady (poll GET /, 100 мс, ранний выход при смерти процесса, bounded `timeout_seconds`), сегменты через newEndpointTranscriber(/inference), Close убивает дерево (cancel → SIGKILL, затем reap). Ошибки однострочные: exited before listening (exit + последняя строка stderr) / did not become ready (поднять timeout_seconds).
- `internal/voice/stt_http.go` — путь эндпоинта параметризован (newEndpointTranscriber); NewHTTPTranscriber не изменил контракт, модель не шлётся, когда сервер держит её сам.
- `internal/voice/config.go` — BackendServer, `server_command` (+ дефолт), decode server, auto: server → command → http через общий resolveLocalSTT.
- `internal/voice/session.go` — ensureTranscriber строит ServerTranscriber; worker после дренирования очереди зовёт retireTranscriber (кэш транскрибера не переживает режим, closable-бэкенды закрываются); loop закрывает queue на каждом выходе (заодно починена доэтапная утечка воркера на Discard/Close); Close ждёт workerDone.
- Тесты: stt_server_test.go (parse, fails-fast, ready-timeout, идемпотентный Close, freeListenPort), TestEndpointTranscriberUsesTheConfiguredPath, session-ретайры на End/Discard + второй режим строит новый бэкенд; всё зелёное под -race.
- doc/voice.md (раздел «The resident server», справочник, троблшутинг) + CHANGELOG [Unreleased].

Верификация: `make fmt-check lint test` зелёные (lint 0 issues); живой smoke через настоящий `voice.Resolve`+`Session` со скриптовым capture: auto → server (ggml-large-v3-turbo), два сегмента через ровно один процесс whisper-server, после End — 0 процессов. Merge координировать с `voice-model-install` (общий резолвер).

**Done (2026-09-04).** Ветка feature/voice-resident-whisper-server, коммит 1725cc7 в worktree (10 файлов, +539/−45); заметка задачи в main — 132dc9c. ServerTranscriber (свободный порт, proc.Start, waitReady с ранним выходом и bounded timeout, POST /inference, Close убивает дерево), HTTPTranscriber параметризован путём, config: server_command + auto server→command→http, session: retireTranscriber на End/Discard/Close + починена утечка воркера. Гейты зелёные (lint 0 issues, -race чисто); живой smoke: auto→server, один резидентный процесс на два сегмента, 0 процессов после End. Merge координировать с voice-model-install (общий резолвер).

## Acceptance Criteria

- voice.stt.backend понимает server: cozyphi запускает whisper-server с найденной моделью на свободном порту 127.0.0.1
- Ленивый старт на первом сегменте, переживает паузы внутри диалог-режима, убивается на End/Discard/Close после дренажа очереди
- Готовность (загрузка модели) ожидается с bounded timeout; ошибка старта — одна строка с подсказкой
- auto резолвится в порядке server → command → http; /voice status называет whisper-server и модель
- Командная строка сервера конфигурируется с плейсхолдерами {model} {port}; дефолт проверен живым запуском на этой машине
- make fmt-check lint test зелёные в worktree задачи; после выхода нет осиротевшего whisper-server

## Verification Plan

1. go test ./internal/voice/... в worktree: резолв бэкенда, lifecycle (Close зовётся на End/Discard), ready-wait таймаут, запрос на /inference
2. Живой smoke: whisper-server поднят cozyphi, второй сегмент не платит за загрузку модели; ps после выхода чист
3. make fmt-check lint test в worktree

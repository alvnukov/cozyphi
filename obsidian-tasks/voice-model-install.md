---
id: voice-model-install
title: 'Голос: вместо ошибки предлагать скачать и настроить модель распознавания'
status: done
priority: high
model_level: high
task_type: feature
tags:
    - voice
    - tui
    - ux
acceptance_criteria:
    - Если whisper-cli есть, а ggml-модели нет, Ctrl+G и /voice показывают предложение скачать модель по умолчанию (small, ~466 MB), а не ошибку «no transcriber configured».
    - Согласие запускает фоновую загрузку с прогрессом в футере; по завершении модель выбрана без перезапуска cozyphi, voice.stt.model записан в config.yaml, показан тост «installed — Ctrl+G to talk».
    - Отказ показывает тост с /voice install и voice.stt.model; без явного согласия или команды ничего не скачивается.
    - Команды /voice models (каталог с размерами, установленные и активная) и /voice install [name] (загрузка и выбор модели) работают; повторный install во время загрузки сообщает прогресс.
    - Загрузка идёт в .part с докачкой по Range, проверяет magic «lmgg» и Content-Length, переименовывает атомарно; обрыв или выход из cozyphi оставляют .part для докачки.
    - 'Подсказки резолвера разделены: нет whisper-cli / нет модели / voice.stt.model указывает на отсутствующий файл; voice.stt.model принимает короткое имя (small) и путь.'
    - Из нескольких установленных моделей без пина выбирается лучшая по рангу каталога, а не первая по алфавиту.
    - specs/voice-models.md, doc/voice.md и CHANGELOG обновлены; make fmt-check lint test зелёные.
verification_plan:
    - 'go test ./internal/voice/... с httptest-сервером: полная загрузка, докачка по Range, битый magic, обрыв (Content-Length), отмена контекста, 404.'
    - Тесты резолвера на три раздельные подсказки, короткое имя в voice.stt.model и выбор модели по рангу.
    - Тесты /voice models и /voice install в internal/tui/commands (fake Host).
    - make fmt-check lint test в воркtree; happ diagnostics по затронутым файлам.
    - 'Ручная проверка: убрать модели из ~/.cozyphi/models, запустить cozyphi, Ctrl+G → предложение → Enter → прогресс → тост → Ctrl+G работает без перезапуска.'
created_at: "2026-09-03T20:49:36.787099Z"
updated_at: "2026-09-04T01:54:47.168104Z"
---

## Body

**Проблема.** Пользователь с установленными ffmpeg и whisper-cli, но без ggml-модели, получает «no transcriber configured — install whisper-cpp and a ggml model…». Homebrew whisper-cpp модель не ставит (только for-tests-ggml-tiny.bin), так что это состояние по умолчанию у любого, кто поставил whisper-cpp. Подсказка вводит в заблуждение, а пользователь хочет, чтобы cozyphi сама сказала «модель не установлена» и предложила скачать и всё настроить.

**Решение.** Каталог моделей whisper.cpp (tiny, base, small, medium, large-v3-turbo, large-v3) с URL на huggingface и приблизительными размерами; загрузчик с докачкой, проверкой и атомарным переименованием; предложение через overlay вопроса (QuestionAskMsg) при Ctrl+G и /voice; фоновая загрузка с прогрессом в футере; после установки — пересчёт Resolved без перезапуска и запись voice.stt.model через harnesssettings; команды /voice models и /voice install [name]; раздельные подсказки резолвера; выбор лучшей модели по рангу.

**Модель по умолчанию — small.** Замер на Apple M2 (jfk.wav, 11 с, весь процесс whisper-cli): small 3,3 с при закреплённом языке и 3,9 с при auto; large-v3-turbo 9,4 с и 16,2 с. Энкодер turbo совпадает с large-v3 и обрабатывает 30-секундное окно на каждый сегмент, поэтому для диалогового режима с короткими сегментами small — единственный разумный дефолт; более точные модели ставятся командой /voice install medium или large-v3-turbo.

**Границы.** Установка whisper-cpp и ffmpeg остаётся на пользователе (подсказка с brew install). Секреты (api_key) не попадают в тосты, логи и сообщения. Ничего не скачивается без Enter в предложении или явной команды. Подробности — в specs/voice-models.md на ветке задачи.

**Follow-up (отдельная задача).** voice.language: auto удваивает задержку (детекция языка — отдельный проход энкодера); стоит закреплять язык после первой детекции в сессии.

## Acceptance Criteria

- Если whisper-cli есть, а ggml-модели нет, Ctrl+G и /voice показывают предложение скачать модель по умолчанию (small, ~466 MB), а не ошибку «no transcriber configured».
- Согласие запускает фоновую загрузку с прогрессом в футере; по завершении модель выбрана без перезапуска cozyphi, voice.stt.model записан в config.yaml, показан тост «installed — Ctrl+G to talk».
- Отказ показывает тост с /voice install и voice.stt.model; без явного согласия или команды ничего не скачивается.
- Команды /voice models (каталог с размерами, установленные и активная) и /voice install [name] (загрузка и выбор модели) работают; повторный install во время загрузки сообщает прогресс.
- Загрузка идёт в .part с докачкой по Range, проверяет magic «lmgg» и Content-Length, переименовывает атомарно; обрыв или выход из cozyphi оставляют .part для докачки.
- Подсказки резолвера разделены: нет whisper-cli / нет модели / voice.stt.model указывает на отсутствующий файл; voice.stt.model принимает короткое имя (small) и путь.
- Из нескольких установленных моделей без пина выбирается лучшая по рангу каталога, а не первая по алфавиту.
- specs/voice-models.md, doc/voice.md и CHANGELOG обновлены; make fmt-check lint test зелёные.

## Verification Plan

1. go test ./internal/voice/... с httptest-сервером: полная загрузка, докачка по Range, битый magic, обрыв (Content-Length), отмена контекста, 404.
2. Тесты резолвера на три раздельные подсказки, короткое имя в voice.stt.model и выбор модели по рангу.
3. Тесты /voice models и /voice install в internal/tui/commands (fake Host).
4. make fmt-check lint test в воркtree; happ diagnostics по затронутым файлам.
5. Ручная проверка: убрать модели из ~/.cozyphi/models, запустить cozyphi, Ctrl+G → предложение → Enter → прогресс → тост → Ctrl+G работает без перезапуска.

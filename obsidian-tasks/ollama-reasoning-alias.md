---
id: ollama-reasoning-alias
title: Ollama's "reasoning" field drops a thinking model's whole turn
status: in_progress
priority: high
task_type: bug
branch: worktree-ollama-reasoning-alias
worktree_path: .claude/worktrees/ollama-reasoning-alias
acceptance_criteria:
    - Стриминговые дельты с "reasoning" накапливаются в ReasoningContent, content доходит целиком
    - Целое сообщение в чанке с "reasoning" декодируется так же
    - При обоих написаниях выигрывает каноническое reasoning_content
    - 'Кодирование не изменено: в запросе по-прежнему только reasoning_content'
verification_plan:
    - go test ./internal/llm/... ./internal/session/... ./internal/agent/...
    - gofmt -l и go vet по изменённым пакетам
    - 'Живая проверка на Ornith 1.5 9B через Ollama 192.168.88.164:11435: reasoning_effort none и high'
created_at: "2026-09-08T20:37:24.935856Z"
updated_at: "2026-09-08T20:37:24.935856Z"
---

## Body

**Bug.** Локальная reasoning-модель через Ollama (`base_url: .../v1`, protocol
openai) приходила в cozyphi как поток пустых content-дельт: весь ход модели
терялся, в UI — пустой ответ.

**Root cause** — `internal/llm/types.go`: cozyphi читал только
`reasoning_content`. OpenAI-совместимые серверы расходятся в написании поля.
Ollama `openai/openai.go` (сервер 0.33.3) объявляет
`Reasoning string json:"reasoning,omitempty"` и на `Message`, и на
стриминговой `Delta`; поля `reasoning_content` в её ответе нет вообще.

**Fix.** `Message` и `StreamDelta` получили `UnmarshalJSON`, принимающий
`reasoning` как алиас. Приоритет взят у эталонной реализации этого эндпоинта,
`@ai-sdk/openai-compatible`: `reasoning_content ?? reasoning`. Кодирование не
тронуто — в запросах по-прежнему уходит только каноническое имя, так что
провайдеры, ожидающие `reasoning_content`, не затронуты.

**Известная асимметрия, не чинится здесь.** На отправке Ollama читает тоже
только `reasoning` (`FromChatRequest` кладёт `msg.Reasoning` в
`api.Message.Thinking`), а все mainstream-клиенты шлют `reasoning_content` —
opencode и AI SDK в том числе. То есть прошлое мышление до Ollama не доезжает
ни у кого. Проверенной реализации, которая делает иначе, нет; решение
отправлять ли дополнительно `reasoning` — за пользователем.

## Acceptance Criteria

- Стриминговые дельты с "reasoning" накапливаются в ReasoningContent, content доходит целиком
- Целое сообщение в чанке с "reasoning" декодируется так же
- При обоих написаниях выигрывает каноническое reasoning_content
- Кодирование не изменено: в запросе по-прежнему только reasoning_content

## Verification Plan

1. go test ./internal/llm/... ./internal/session/... ./internal/agent/...
2. gofmt -l и go vet по изменённым пакетам
3. Живая проверка на Ornith 1.5 9B через Ollama 192.168.88.164:11435: reasoning_effort none и high

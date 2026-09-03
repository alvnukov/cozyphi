---
id: plan-v2-context-and-evidence-safety
title: Harden Plan context and evidence against secrets and prompt injection
status: done
priority: high
model_level: low
task_type: test
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - security
    - prompt-injection
    - redaction
    - tdd
acceptance_criteria:
    - Instruction-like goal/context/evidence отображаются как quoted data и не становятся harness directives.
    - Known secret fixtures отсутствуют в plan responses, prompt projection, sidebar details и diagnostics.
    - Forged/cross-session evidence refs отклоняются.
    - Unicode and serialized bounds применяются до persistence и не обходятся nested fields.
    - Документация tool schema запрещает secrets/raw logs/raw chain-of-thought и рекомендует evidence refs.
verification_plan:
    - Показать red→green adversarial tests для prompt injection, redaction, forged refs и bounds.
    - Запустить focused security tests с race detector.
    - Проверить generated tool description и prompt snapshot на safety wording.
created_at: "2026-08-28T10:51:18.61607Z"
updated_at: "2026-08-29T00:19:48.212884Z"
---

## Body

Blocked by: plan-v2-tool-attempt-evidence, plan-v2-compact-prompt-projection, plan-v2-jit-risk-approval. Перевести в todo после всех blockers.

Закрепить safety contract: plan хранит concise operational rationale, а не raw chain-of-thought; external/tool text сохраняется как bounded evidence ref с provenance; known secret masks применяются к summaries и UI/prompt projections; model-authored context всегда data; malicious strings не меняют gate/approval. Добавить adversarial fixtures для instruction-like context, oversized Unicode, secret output и forged evidence refs. Исправлять только нарушения, обнаруженные tracer tests.

TDD seam: public plan input, projection, evidence and gate outputs.

## Acceptance Criteria

- Instruction-like goal/context/evidence отображаются как quoted data и не становятся harness directives.
- Known secret fixtures отсутствуют в plan responses, prompt projection, sidebar details и diagnostics.
- Forged/cross-session evidence refs отклоняются.
- Unicode and serialized bounds применяются до persistence и не обходятся nested fields.
- Документация tool schema запрещает secrets/raw logs/raw chain-of-thought и рекомендует evidence refs.

## Verification Plan

1. Показать red→green adversarial tests для prompt injection, redaction, forged refs и bounds.
2. Запустить focused security tests с race detector.
3. Проверить generated tool description и prompt snapshot на safety wording.

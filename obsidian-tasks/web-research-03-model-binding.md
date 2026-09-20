---
id: web-research-03-model-binding
title: 03 — Configure an explicit pinned web model without fallback
status: done
priority: high
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
branch: feature/web-research-03-model-binding
worktree_path: .worktrees/web-research-03-model-binding
acceptance_criteria:
    - The user selects a concrete provider/endpoint/account/model/effective configuration using existing configuration identities.
    - Save/reload preserves the selection; missing or invalid references are not-ready rather than session-model fallback.
    - Changes invalidate readiness for the old binding and never erase existing material restrictions.
verification_plan:
    - Save/reload configured and unset choices through the actual configuration/settings boundary.
    - Change account, endpoint, model and effective options; assert readiness invalidation and no default substitution.
    - Run only affected configuration/settings tests and scoped gates; record captured identity with credentials removed.
created_at: "2026-09-19T19:10:37.769516Z"
updated_at: "2026-09-20T07:59:25.087931Z"
---

## Body

**What to build:** An explicit web-model setting visible to the user, without claiming that the chosen model has passed capability checks.

**Blocked by:** [02](web-research-02-not-ready.md).

**Contract:** [Spec](../specs/protected-web-research.md), D2, Q9/Q25, T01. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Reuse existing model configuration references and settings/save behavior. Resolve an effective binding at admission, not a display label. Report unset, removed and changed configurations through the existing not-ready path. Mark pending checks/results for invalidation when the effective route changes. Keep authentication transport-owned; never serialize tokens into model-facing identity. Add settings round-trip and pin tests.

**Do not change:** No hard-coded provider, automatic model selection, live preflight, capability invention or new provider protocol. This task does not enable real research.

**Proof required:** Settings save/reload demonstration and captured resolved identities for two distinct configurations; missing reference and changed endpoint/account must refuse reuse. Assert no main-model substitution and no secret fields in displayed errors.

**Stop condition:** If existing config identity cannot represent the effective recipient without protocol changes, document the specific missing contract rather than guess fields.

**Reopened (2026-09-20).** Блокер 02 снят (PR #43 в main).

**Started (2026-09-20).** Беру в работу: явный web-модельный биндинг без фолбэка, маршрут как у 02.

**Done (2026-09-20).** PR #45 влит squash-коммитом 42c1e9d1 в main. web.model пинит имя записи из models:; биндинг резолвится на admission (unset/missing/resolved), identity/fingerprint без APIKey, отказ называет причину, fallback на сессионную модель отсутствует; tool остаётся fail-closed до capability-preflight. Гейты scoped, линтер 0 issues.

## Acceptance Criteria

- The user selects a concrete provider/endpoint/account/model/effective configuration using existing configuration identities.
- Save/reload preserves the selection; missing or invalid references are not-ready rather than session-model fallback.
- Changes invalidate readiness for the old binding and never erase existing material restrictions.

## Verification Plan

1. Save/reload configured and unset choices through the actual configuration/settings boundary.
2. Change account, endpoint, model and effective options; assert readiness invalidation and no default substitution.
3. Run only affected configuration/settings tests and scoped gates; record captured identity with credentials removed.

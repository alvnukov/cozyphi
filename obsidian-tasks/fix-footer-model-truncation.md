---
id: fix-footer-model-truncation
title: Футер режет имя модели на дефисе вместо аккуратного усечения
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - ui
    - footer
    - phase1-ui
created_at: "2026-08-23T20:00:07.078717Z"
updated_at: "2026-08-23T23:27:47.121358Z"
---

## Body

На скриншотах футер обрезает имя модели на дефисе ("deepseek-v4-pro-") — строка статуса не умеет усекаться по ширине терминала. Починить усечение футера: ellipsis или умный склей (модель коротко, остальное по приоритету), проверить на узких терминалах (60–80 колонок). Смотреть internal/components футер-виджет.

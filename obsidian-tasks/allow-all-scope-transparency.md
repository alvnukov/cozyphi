---
id: allow-all-scope-transparency
title: Allow-All options do not say what they allow
status: done
tags:
    - ui
    - ux-standard
    - bug
created_at: "2026-09-01T11:15:10.000000Z"
updated_at: "2026-09-01T16:45:00.000000Z"
---

## Body

**Проблема.** Опции «Allow All for This Session» и «Allow All for Every Session» (`internal/tui/overlays/overlays.go:579-580`) не говорят, ЧТО именно будет разрешено: весь инструмент? команду с этими аргументами? любой bash? Пользователь выбирает вслепую. При этом «Every Session» (:386-387, `AllowPersistent`) молча пишет правило на диск — постоянное расширение прав без дополнительного подтверждения и без указания, в какой файл оно легло.

**Что сделать.** Подписать каждую опцию конкретным правилом, которое она создаст (например «Allow All for This Session — bash: любые команды до конца сессии»); для persistent-варианта — либо inline-подтверждение в стиле y/n из стандарта (см. Deletion and confirmation в internal/tui/DESIGN.md), либо как минимум тост с именем файла настроек и созданным правилом, чтобы след был виден и правило было легко найти и убрать.

**Критерий.** Из текста опции однозначно ясно будущее правило; после persistent-выбора пользователь знает, где оно лежит. Связано: [[ask-overlay-detail-reach]].

**Результат.** Сделано (commit c2a3d84, merge в main). Выяснилось при трассировке: обе опции вовсе не создают скоуп-правило — AllowSession глушит весь permission-гейт (`c.allowAll`) до выхода, AllowPersistent пишет `permissions.dangerously_allow_all: true` в глобальный `~/.cozyphi/config.yaml`, причём ошибка записи молча глоталась (`_ =`). Теперь: (1) под опциями строка-пояснение, следующая за выделением (двухтактный жест «выбрать → активировать» читает мелкий шрифт между тактами): у Approve — «этот вызов один раз», у обеих Allow-All — warning-стиль с честным охватом, persistent называет ключ правила и файл (`PermissionAskMsg.PersistPath` заполняет контроллер, показ через `pathutil.ShortPath`, без проекта — «the global config»). (2) Persistent-выбор не резолвится сразу: он взводит стандартный y/n-вопрос из кита (`browse.Confirm`, раздел Deletion and confirmation) с именем файла; `y` пишет, `n`/Esc снимают вопрос не закрывая ask, любая другая клавиша/колесо/клик снимают и действуют (клик по persistent-опции взводит заново — мышиный путь проходит через тот же вопрос). (3) Контроллер публикует `PermissionPersistedMsg{Path, ErrText}` после попытки записи — editor показывает тост «Allow-all rule written to …» или ошибку: правило, в существование которого пользователь верит зря, хуже ошибки. Тесты: allowall_test.go (пояснения по кольцу, arm/y/n/Esc/withdraw-and-act, мышиный arm и снятие колесом). Контракт — в DESIGN.md (Choice modals) и CHANGELOG.

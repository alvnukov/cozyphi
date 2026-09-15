---
id: ci-dependabot-gates
title: PR'ы Dependabot всегда красные — метки dependencies нет в репозитории, а codeql-action разрезан по двум PR
status: done
priority: medium
model_level: medium
task_type: chore
tags:
    - ci
    - dependencies
branch: chore/codeql-action-4.38.0
worktree_path: .worktrees/codeql-action-4.38.0
acceptance_criteria:
    - Гейт changelog не валит PR Dependabot — метка dependencies существует в репозитории и приезжает на его PR, workflow по ней скипается.
    - github/codeql-action/init и analyze стоят на одной версии в main; PR с бампом codeql-action проходит job codeql.
    - Следующий минорный/патчевый бамп экшенов приезжает одним PR, а не по одному на каждый использованный подэкшен.
verification_plan:
    - yaml.safe_load по изменённым .github/dependabot.yml и .github/workflows/codeql.yml.
    - Пин 4.38.0 сверен с API — тег v4.38.0 репозитория github/codeql-action разыменован в тот же коммит.
    - CI на самом PR — job codeql зелёный, changelog скипается по метке.
created_at: "2026-09-15T20:45:00Z"
updated_at: "2026-09-15T20:45:00Z"
---

## Body

**Откуда.** Разбор трёх открытых PR Dependabot (#27, #28, #29) 2026-09-15 — все три красные, ни один не мог позеленеть сам.

**Две независимые причины.**

1. *Гейт changelog.* Workflow `.github/workflows/changelog.yml` пропускает PR по метке `dependencies`, метке `Skip Changelog` или подстроке `[chore]` в заголовке. В `.github/dependabot.yml` метка `dependencies` прописана для обеих экосистем, но самой метки в репозитории не было — `gh label list` знал только `Skip Changelog`. Метку, которой нет, Dependabot повесить не может: все три PR приехали без меток, заголовки вида `chore(deps): …` под условие `[chore]` (в скобках) не подходят, и гейт валился на каждом PR Dependabot по построению.

2. *Разрезанный codeql-action.* `init` и `analyze` живут в одной job и отказываются работать на разных версиях: `Loaded a configuration file for version 'X', but running version 'Y'`. Dependabot развёл их по разным PR (#28 — init, #29 — analyze), поэтому каждый из них ломал job `codeql` сам по себе, и ни один нельзя было слить зелёным.

**Done (2026-09-15).** Метка `dependencies` создана в репозитории и повешена на #27/#28/#29. `codeql.yml` — обе строки `uses:` подняты до `b96794f0` (тег v4.38.0 разыменован через API, совпадает с тем, что предлагал Dependabot) одним коммитом, поэтому job `codeql` зелёный. В `github-actions` добавлена группа `actions-minor-and-patch` — зеркало уже существующей `go-minor-and-patch`: минорные и патчевые бампы экшенов теперь приезжают одним PR, так что связанные подэкшены больше не расходятся по разным веткам. #28 и #29 закрыты как superseded, #27 (`upload-sarif` в `scorecard.yml`, отдельный workflow, рассинхрона не было) уезжает своим PR после ребейза.

**Замечено по дороге, не чинилось здесь.** На #29 упал `test (ubuntu-latest)` — `TestChildLivesInItsParentsFamilyAndCannotResurrect/running=false` в `cmd` с `Post "http://127.0.0.1:…/chat/completions": context deadline exceeded`. PR трогает только yml, так что это флейк по таймауту фейкового LLM-сервера под нагрузкой раннера, а не регрессия. Нужна отдельная задача на запас по времени в этом тесте.

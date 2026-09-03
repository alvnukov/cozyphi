---
id: cozyphi-branding
title: Readable CozyPhi wordmark, fork version v0.1.0, update check → CozyPhi repo
status: done
priority: high
task_type: chore
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - branding
    - version
created_at: "2026-08-23T18:11:33.508572Z"
updated_at: "2026-08-23T18:13:31.429549Z"
---

## Body

Follow-up to tui-static-splash: (1) the ANSI-shadow wordmark's z read as T ("COTYPHI"); switch the wordmark to figlet-standard glyphs with an unambiguous z. (2) Start the fork's own version line at v0.1.0 (was upstream v0.16.0). (3) Update check and `phi update` still targeted upstream pulseaiclub/phi releases — with v0.1.0 the footer would nag "update available v0.16.0" forever; point Repo at alvnukov/CozyPhi (empty for now → check silently no-ops until the fork publishes releases).

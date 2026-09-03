---
id: tui-static-splash
title: Replace animated splash sphere with static CozyPhi wordmark
status: done
priority: high
task_type: feature
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - splash
    - ux
created_at: "2026-08-23T17:59:10.655883Z"
updated_at: "2026-08-23T18:05:39.663062Z"
---

## Body

The empty-transcript welcome screen renders an animated noise-lit ASCII sphere at 30 fps (splash.Sphere + valuenoise + WakeIn loop in transcript pane). User verdict: annoying. Replace with a static CozyPhi wordmark (no animation, no wakeups): centered logo, tagline + version line, Ctrl+K / ! help line, hint. Delete sphere.go, valuenoise.go, their tests; drop the animation wiring from TranscriptPane.

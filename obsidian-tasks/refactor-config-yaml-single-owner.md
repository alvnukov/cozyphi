---
id: refactor-config-yaml-single-owner
title: Three uncoordinated writers of ~/.cozyphi/config.yaml silently clobber each other
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - architecture
    - config
    - review-2026-08
created_at: "2026-08-27T16:09:20.837791Z"
updated_at: "2026-08-27T21:47:57.963741Z"
---

## Body

project.parseConfigFile (internal/project/config.go:160-166, planConfig:229) parses plan.defaults; harnesssettings.Manager (manager.go:86, lookupPath doc 'plan' 'defaults') re-parses and rewrites the same key with its own optimistic token; project.SetDangerouslyAllowAll (config.go:380-451) does a third line-based rewrite with no lock or token. A SetDangerouslyAllowAll write between Apply's readDocument and writeAtomicOwnerOnly is silently lost (and vice versa). Fix: one owner for config.yaml writes; project must not parse plan.defaults independently of harnesssettings. NOTE: harnesssettings is part of the in-flight plan-runtime feature - re-verify file:line after it lands.

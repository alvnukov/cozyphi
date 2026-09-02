---
id: queue-inplace
title: Queued message stays in place until delivered to the model
status: done
---

# Queued message stays in place until delivered

User report: a message submitted mid-run used to "float up" with a (queued)
hint while the model ignored it. Required UX: the row stays exactly where it
was submitted; the (queued) hint clears at the moment the model receives the
message (mid-turn boundary or next-turn dequeue), so the transcript shows
when delivery happened. New model content may only render below the row
after delivery.

## Acceptance

- Editor-level integration test: row index unchanged from submit through
  delivery; hint cleared before any round-2 content renders below it;
  exactly one user row with the queued text (no duplicate from the engine's
  injected append).

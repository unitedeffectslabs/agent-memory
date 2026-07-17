# Bug: watcher swallows Create events on Linux (debounce replaces instead of merges)

**Date found:** 2026-07-16
**Found during:** local-embeddings epic, Phase 5 Linux verification (first-ever test run on Linux)
**Status:** Open — documented, not yet fixed
**Severity:** Low today (no user-visible breakage), latent correctness risk

## Symptom

`go test ./internal/watcher/` fails deterministically on Linux (3/3 runs, Docker
`golang:1.26-bookworm`, arm64):

```
--- FAIL: TestOnCreate (2.01s)
    fswatcher_test.go:121: expected OnCreate to be called
```

`TestOnModify` and `TestOnDelete` pass. The full suite is green on macOS.

## Root cause

Writing a new file on Linux (inotify) emits **two events** for the same path within
milliseconds: `CREATE`, then `WRITE`. In `internal/watcher/fswatcher.go`,
`handleEvent` → `debounce(path, fn)` stores **one pending closure per path and
replaces it** on each new event (`fswatcher.go:114-128`): the `WRITE` event cancels
the `CREATE` timer and substitutes a closure that only sees `WRITE`. When the
debounce fires, the handler classifies the event as a modification —
`handler.OnCreate` is never called for newly written files.

macOS (fsnotify kqueue/FSEvents backend) delivers/coalesces these events differently,
so the swallow never manifests there.

## Impact

- **Today: effectively none for users.** `engine.OnCreate` and `engine.OnModify`
  are identical (`engine.go:717-747` — both call `IndexFile`), so new files on
  Linux are still indexed, merely misclassified as modifications.
- **Latent risk:** any future divergence between create and modify handling
  (e.g. create-only bookkeeping, activity-log semantics, per-event UX) silently
  breaks on Linux only.
- Blocks a fully green `go test ./...` on Linux (Phase 5 verify gate) until fixed.

## Suggested fix (not applied)

In the debouncer, **accumulate the fsnotify op bits per path** instead of replacing
the closure — e.g. keep `pendingOps map[string]fsnotify.Op`, OR-ing each event's op;
when the timer fires, classify with precedence Remove/Rename > Create > Write/Chmod
from the merged bits, then clear the entry. Semantics on macOS are unchanged
(single-op case degenerates to today's behavior); Linux create+write merges to
Create. `TestOnCreate` then passes on both platforms.

Fix belongs in its own small PR (per working rules); the watcher is outside the
local-embeddings epic's scope ("explicitly unchanged" list).

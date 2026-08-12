# Epic: Background Presence (cross-platform tray, start-on-login, single instance)

**Date:** 2026-08-12
**Status:** Proposed — definition requested in PR #2 review; first in the post-merge epic queue
**Owner:** Bo Motlagh (definition scoped by Bo; drafted by Nestor per review)

## Goal

Close the product gap the close=quit triage left on Windows/Linux: **the app only watches
and indexes while its window is open.** On macOS the status-bar tray keeps the app resident
in the background; on Windows/Linux, closing the window now stops watching entirely (stdio
search keeps working against the existing index, but the index goes stale until the next
launch). Windows has a first-class notification-area pattern for exactly this — the current
limitation is only that `tray.go` is darwin-only Objective-C.

After this epic: on every platform that supports it, closing the window hides the app into
a tray with a Show/Quit menu and watching continues; the app can optionally start on login;
and running two instances against one DB is structurally impossible.

## User-set constraints (settled in the PR #2 review — do not relitigate)

- Cross-platform tray/notification-area icon with **Show / Quit**, restoring hide-on-close
  on platforms that have a tray.
- **Start-on-login option**, per-OS mechanism (macOS Login Items / Windows registry Run key /
  Linux XDG autostart).
- **Linux reality check is part of the epic**: tray support varies by desktop environment
  (appindicator vs legacy tray protocols). Document what we target and what degrades to
  close=quit — degradation is acceptable, silence about it is not.
- **Single-instance guard** so the zombie-stacking class of bug is structurally impossible
  regardless of tray state.
- Current **close=quit stays as the documented fallback** wherever a tray isn't available.

## Execution Notes (read first if you are the implementing session)

- Follow the conventions proven by `local-embeddings.md`: phase gates with explicit
  **Verify** steps, record deviations in this file, never start a phase before the prior
  gate passes.
- The `hasTray` seam already exists: `tray.go` (darwin) / `tray_stub.go` (!darwin) each own
  the constant, and `main.go` keys `HideWindowOnClose` off it. This epic's tray
  implementations replace the stub per platform and flip `hasTray` there — `main.go` should
  need no changes for the hide-on-close behavior.
- Shutdown correctness is already handled (engine.Stop-before-Close + atomic index writes,
  PR #2 revision round). Tray-Quit must go through the same shutdown path.

## Research the epic must settle (Phase 0)

1. **Library evaluation — likely `fyne-io/systray`**: the known hard question is message-loop
   integration with Wails v2 (both want the main thread on some platforms). Spike: tray icon +
   Show/Quit alongside a running Wails window on Windows first, then Linux. Record findings
   here the way local-embeddings recorded its Phase 0 numbers. Fallback candidates if it
   fails: platform-native minimal implementations (Win32 Shell_NotifyIcon via CGo; keep the
   existing darwin ObjC).
2. **Linux tray matrix**: GNOME (needs appindicator extension), KDE, XFCE, Pop!_OS — what
   works out of the box, what needs a package, what doesn't work at all. Output: a support
   table in this doc + the degrade-to-close=quit list.
3. **Single-instance mechanism**: evaluate a lock file beside the DB (flock/LockFileEx) vs a
   local socket. Must handle stale locks after crashes; second launch should surface the
   existing instance's window when possible rather than just erroring.
4. **Start-on-login mechanics** per OS, including uninstall/cleanup behavior (removing the
   app must not leave a broken login entry).

## Phases (outline — implementation plan written after Phase 0, per workflow)

- **Phase 0 — Spike (throwaway):** systray×Wails coexistence on Windows + one Linux DE.
  **Gate:** tray icon + working Show/Quit alongside a live Wails window, findings recorded here.
- **Phase 1 — Single-instance guard** (independent of tray; kills the zombie class on its
  own). **Verify:** second launch on each OS surfaces/exits cleanly; crash + relaunch does
  not deadlock on a stale lock.
- **Phase 2 — Windows tray** (flip `hasTray` on windows, hide-on-close restored, Quit goes
  through full shutdown). **Verify:** close → still watching (index a file while hidden);
  Quit → process exits, no zombies.
- **Phase 3 — Linux tray** per the support matrix, with documented degradation.
- **Phase 4 — Start-on-login** (all platforms, off by default, Settings toggle).
- **Phase 5 — Docs + epic close-out** (README/FEATURES/ARCHITECTURE; record measured
  behavior per platform).

## Out of scope

- Any change to indexing/search behavior (this epic is presence/lifecycle only).
- macOS tray rework (existing ObjC tray stays; only refactor if the Phase 0 library choice
  makes unification free).
- Auto-update, menubar richness beyond Show/Quit (parking lot).

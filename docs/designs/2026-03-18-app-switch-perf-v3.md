# App-Switching Performance v3: Remove Hot-Path Logging & Direct Window Server Activation

**Date:** 2026-03-18
**Status:** Approved

## Problem

After v2 (unified TapContext with cached app state, c90af7a), two bottlenecks remain:

1. **Per-keystroke debug logging**: `log.debug("tap: ...")` fires on every keyDown event system-wide. `OSLogMessage` construction with hex formatting has non-trivial overhead even in release builds.
2. **Apple Event activation latency**: `NSRunningApplication.activate()` uses Apple Events (~10-50ms). Direct window server activation via SkyLight private SPI is ~1-5ms.

## Decision Log

**1. Remove debug logging from event tap hot path?**
- Options: A) Keep as-is · B) Gate behind `#if DEBUG` · C) Remove entirely
- Decision: **C) Remove entirely** — Fires on every keyDown system-wide. Developers can re-add temporarily when debugging.

**2. Use private SkyLight SPI for app activation?**
- Options: A) Keep `NSRunningApplication.activate()` · B) Use `_SLPSSetFrontProcessWithOptions` via `dlsym` with fallback
- Decision: **B)** — Direct window server IPC bypasses Apple Event Manager (~10-30ms saved per switch). Loaded via `dlsym` with graceful fallback. Safe for non-App Store distribution (HotApp already requires Accessibility + CGEvent taps).

**3. Pre-unhide hidden apps separately?**
- Options: A) Let `activate()` handle it · B) Call `unhide()` before `activate()`
- Decision: **A)** — `activate()` on macOS 14+ handles unhiding. Extra call adds overhead without benefit.

**4. Lock-free atomics for `paused` flag?**
- Options: A) Keep inside shared lock · B) Separate atomic
- Decision: **A)** — Lock hold time is sub-microsecond. YAGNI.

## Changes

All in `Sources/HotApp/Services/ShortcutManager.swift`:

1. **Remove `log.debug` from tap callback** — The line that fires on every keyDown event is removed entirely.

2. **Add SkyLight fast activation** — Module-level lazy constant loads `_SLPSSetFrontProcessWithOptions` from SkyLight framework via `dlsym`. Thread-safe one-time init guaranteed by Swift.

3. **Add `_fastActivate(_ app:)` helper** — Extracts PID from `NSRunningApplication`, converts to PSN via `GetProcessForPID`, calls SkyLight SPI. Falls back to `app.activate()` if any step fails (dlsym, GetProcessForPID, or SPI call).

4. **Update `_executeToggle`** — `.activate` case calls `_fastActivate` instead of `app.activate()`.

## Fallback Chain

```
dlsym(_SLPSSetFrontProcessWithOptions) fails → app.activate()
GetProcessForPID fails                       → app.activate()
_SLPSSetFrontProcessWithOptions fails        → app.activate()
```

# App-Switching Performance v2: Eliminate Per-Keypress IPC

**Date:** 2026-03-18
**Status:** Approved

## Problem

After the O(1) lookup optimization (5e5fabd), two bottlenecks remain in `_toggleApp()`:

1. **Per-keypress system calls**: `NSWorkspace.shared.frontmostApplication` (Mach message) and `NSRunningApplication.runningApplications(withBundleIdentifier:)` (Launch Services query) are called on every hotkey press (~20-50us combined).
2. **Tap disabled silently**: When macOS disables the event tap via `tapDisabledByTimeout`, hotkeys stop working until the next `refreshRegistrations()` call. No automatic re-enable.

## Decision Log

**1. Cache running app state to eliminate per-keypress IPC?**
- Options: A) Keep system calls per keypress · B) Cache running apps + frontmost via workspace notifications
- Decision: **B)** — Notifications (didLaunch/didTerminate/didActivateApplication) keep cache fresh. Hot path becomes pure memory lookups.

**2. Re-enable tap on tapDisabledByTimeout?**
- Options: A) Rely on refreshRegistrations() · B) Re-enable immediately in callback
- Decision: **B)** — Critical reliability fix. Without this, hotkeys silently break.

**3. Unified TapContext vs separate AtomicLookup?**
- Options: A) Keep separate AtomicLookup + free function · B) Single TapContext with shortcut table + app state + tap port
- Decision: **B)** — Single lock, single context. Lock hold time stays sub-microsecond (dictionary lookups only).

**4. Multiple instances of same bundle ID?**
- Options: A) Store arrays · B) Store latest, re-query on terminate · C) Ignore
- Decision: **B)** — On terminate, check if other instances exist (infrequent). On keypress, check isTerminated as safety net.

## Changes

All in `Sources/HotApp/Services/ShortcutManager.swift`:

1. **Replace `AtomicLookup` with `TapContext`** — holds shortcut table, running app cache, frontmost app, tap port, and pause flag under a single `OSAllocatedUnfairLock`
2. **`matchAndResolve()` method** — returns a `ToggleAction` enum (.hide/.activate/.launch) using only cached state. No system calls.
3. **Re-enable tap on `tapDisabledByTimeout`** — callback re-enables immediately via stored tap port reference
4. **Workspace observers in `ShortcutManager`** — update running app cache on didLaunch/didTerminate/didActivateApplication. Seed initial state on init.
5. **Remove `_toggleApp` free function** — replaced by `_executeToggle` that acts on a `ToggleAction`

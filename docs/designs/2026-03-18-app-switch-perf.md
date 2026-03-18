# Improve App-Switching Performance

**Date:** 2026-03-18
**Status:** Approved

## Problem

The current hotkey-to-app-switch flow has three latency sources:
1. **O(n) linear scan** in `AtomicLookup.match()` — holds unfair lock longer with more shortcuts
2. **`DispatchQueue.main.async` hop** — adds a full runloop cycle (~8-16ms) between keypress detection and app activation
3. **`NSWorkspace.openApplication(at:configuration:)`** used for already-running apps — goes through Launch Services, which is heavyweight for apps already in memory

## Decision Log

**1. Use dictionary for O(1) shortcut lookup**
- Options: A) Keep linear scan · B) Dictionary keyed on (keyCode, modifiers)
- Decision: **B)** — O(1) vs O(n), shorter lock hold time

**2. Eliminate main thread dispatch for running apps**
- Options: A) Keep DispatchQueue.main.async · B) Call activate/hide directly from event tap thread
- Decision: **B)** — `NSRunningApplication.activate(options:)` and `.hide()` are thread-safe (Apple Event transport). Saves ~8-16ms per keypress.

**3. Use NSRunningApplication.activate() for running apps**
- Options: A) Keep openApplication for all cases · B) activate() for running, openApplication for cold launch
- Decision: **B)** — `activate(options:)` is a direct Mach message, nearly instant. `openApplication` reserved for cold launches only.

## Changes

All changes in `Sources/HotApp/Services/ShortcutManager.swift`:

1. **AtomicLookup**: Replace `[ShortcutEntry]` with `[ShortcutKey: ShortcutEntry]` dictionary
2. **Event tap callback**: Look up running apps, call `activate()`/`hide()` directly — no main thread dispatch
3. **Cold-launch fallback**: Only dispatch to main for `NSWorkspace.openApplication` when app isn't running
4. **Split toggleApp**: Non-isolated free function for hot path, main dispatch only for cold launch

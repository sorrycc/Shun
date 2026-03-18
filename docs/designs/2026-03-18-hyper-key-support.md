# Hyper Key / Combo Key Support via CGEvent Tap

**Date**: 2026-03-18
**Status**: Approved

## Problem

The current implementation uses `KeyboardShortcuts.onKeyUp()` which relies on Carbon's `RegisterEventHotKey` API for hotkey dispatch. This works for physical modifier keys but can fail with Karabiner-Elements hyper key (Caps Lock remapped to ⌃⌥⇧⌘), because Karabiner operates at the IOKit/CGEvent level and the Carbon event system may not recognize synthetic modifier events.

## Solution

Replace Carbon-based dispatch with a CGEvent tap in `ShortcutManager`. Keep `KeyboardShortcuts.Recorder` for the recording UI and `KeyboardShortcuts.getShortcut(for:)` to read stored combos from UserDefaults.

## Files Changed

| File | Change |
|---|---|
| `ShortcutManager.swift` | Major rewrite — replace `KeyboardShortcuts.onKeyUp/disable` with CGEvent tap |
| `AppDelegate.swift` | Add Accessibility permission check on launch |
| `ShortcutStore.swift` | Remove `KeyboardShortcuts.disable()` from `remove()` — delegate to ShortcutManager |

## Files NOT Changed

- `AppShortcut.swift` — model stays the same, `keyboardShortcutName` still used for Recorder
- `ShortcutListView.swift` — Recorder UI unchanged
- `Package.swift` — still depends on KeyboardShortcuts for Recorder

## ShortcutManager Rewrite

### Core mechanism

A single CGEvent tap listening for `.keyDown` events. On each event, extract `keyCode` + modifier flags, match against a lookup table built from stored shortcuts.

```
CGEvent tap (.keyDown)
  -> extract keyCode + modifierFlags (masked to ctrl/opt/shift/cmd only)
  -> lookup in [keyCode: [(modifiers, bundleIdentifier, bundleURL)]]
  -> match found? -> toggleApp() + return nil (consume event)
  -> no match? -> return event (pass through)
```

### Key details

- **Active tap** (`CGEvent.tapCreate` with `.defaultTap`) — can consume events
- **Trigger on keyDown** (not keyUp) — more reliable with Karabiner, which can drop keyUp when releasing synthetic modifiers
- **Modifier masking**: Compare only `[.maskCommand, .maskShift, .maskControl, .maskAlternate]`, ignore capsLock/numericPad/function flags. This is what makes Karabiner hyper keys work — their events may carry extra flags.
- **Lookup table rebuilt on `refreshRegistrations()`** — reads each shortcut's combo via `KeyboardShortcuts.getShortcut(for: name)`, which returns `carbonKeyCode` + `carbonModifiers`
- **Run loop**: CGEvent tap port added to a background thread's run loop (standard pattern), callback dispatches `toggleApp()` to `@MainActor`
- **globalEnabled**: when disabled, disable the tap via `CGEvent.tapEnable(tap:enable:)`

### Pause during recording

The CGEvent tap is lower-level than the Recorder's `LocalEventMonitor`. If the tap is active while the user is recording a new shortcut, it consumes the keypress before it reaches the Recorder. The tap callback must check `KeyboardShortcuts.isPaused` and pass events through (return the event unchanged) when true.

### Ignore key repeats

macOS sends repeated `.keyDown` events when a key is held. Without filtering, holding hyper+X would rapidly toggle the app on/off. The tap callback must check `CGEvent.getIntegerValueField(.keyboardEventAutorepeat)` and skip repeat events (return the event unchanged).

### Tap resilience

macOS silently disables CGEvent taps if they take too long processing events. Two safeguards:

- If `CGEvent.tapCreate` returns nil (e.g., Accessibility permission not granted), fall back to the original Carbon path (`KeyboardShortcuts.onKeyUp`) so the app still works for physical modifier keys.
- Periodically check `CGEvent.tapIsEnabled()` and re-enable if macOS disabled it. A simple approach: check and re-enable at the top of each `refreshRegistrations()` call.

### Thread safety

The tap callback runs on a background run loop thread, but `refreshRegistrations()` rebuilds the lookup table on `@MainActor`. Use `os_unfair_lock` or an atomic swap (e.g., build the new table on main, then atomically replace the pointer) so the background reader never sees a half-built table.

## Accessibility Permission

CGEvent taps (active, consuming) require Accessibility permission. Add to `AppDelegate`:

- On launch, check `AXIsProcessTrusted()`
- If not trusted, show an alert prompting the user to grant access in System Settings > Privacy & Security > Accessibility
- Optionally call `AXIsProcessTrustedWithOptions` with prompt flag to trigger the system dialog

## ShortcutStore Cleanup

`KeyboardShortcuts.reset()` in `ShortcutStore.remove()` stays but is now purely a "clear the stored combo from UserDefaults" operation — it no longer implicitly unregisters a Carbon hotkey (since we're no longer using Carbon for dispatch).

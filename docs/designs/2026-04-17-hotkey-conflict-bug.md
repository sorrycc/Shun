# Hotkey Conflict Bug Fix

**Date**: 2026-04-17
**Status**: Proposed

## Problem

When a user records a hotkey combo that is already assigned to another app in Shun's shortcut list, the reassignment silently fails. The UI may appear to accept the new combo but:

- The old row keeps its combo.
- The new row does not become functional.
- Pressing the combo does nothing (or still triggers the old owner).
- No user-visible feedback indicates what went wrong.

User report: *"now I pressed the key assigned to other, nothing happened"* / *"it's not reassigned, just keep the origin state"*.

## Root Cause

`Sources/Shun/Views/ShortcutListView.swift:279-286`:

```swift
private var recorder: some View {
    KeyboardShortcuts.Recorder(for: shortcut.keyboardShortcutName)
        .onChange(of: KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName)) {
            _, newValue in
            resolveConflicts(newShortcut: newValue)
            NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
        }
}
```

SwiftUI's `.onChange(of:)` only re-evaluates its `of:` expression when the enclosing view body re-renders. `KeyboardShortcuts.getShortcut(...)` is a plain `UserDefaults` read — SwiftUI does **not** observe UserDefaults mutations.

When `KeyboardShortcuts.Recorder` commits a new combo it writes UserDefaults directly (see `RecorderCocoa.swift:313-316` `saveShortcut`). That write does not trigger a re-render of `ShortcutRow` (whose `AppShortcut` prop has no combo field). Therefore:

1. The closure passed to `.onChange(of:)` never (reliably) fires.
2. `resolveConflicts()` does not run → the old owner's combo is not cleared. Both `Name`s now hold the same combo in UserDefaults.
3. `.shortcutsDidChange` is never posted → `ShortcutManager.refreshRegistrations()` does not run → the CGEvent tap's lookup table stays stale.
4. The new combo has no effect at runtime.

The library itself does **not** reject duplicate combos across different `Name`s (verified in `RecorderCocoa.swift:272-299` — it only rejects main-menu and system-reserved combos). So there is no library-level safeguard here either; the loss happens purely in Shun's own observation layer.

## Fix

Use the library's own `onChange:` callback (defined in `Recorder.swift:96-105`), which is invoked reliably every time the Recorder commits a recording or clears via delete.

```swift
// Sources/Shun/Views/ShortcutListView.swift:279-286
private var recorder: some View {
    KeyboardShortcuts.Recorder(for: shortcut.keyboardShortcutName) { newValue in
        resolveConflicts(newShortcut: newValue)
        NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
    }
}
```

Delete the `.onChange(of:)` modifier; pass the closure directly to the `Recorder` initializer.

### Why this works

`KeyboardShortcuts.Recorder` calls the supplied `onChange` inside its `saveShortcut(_:)` implementation (`RecorderCocoa.swift:313-316`), which runs synchronously when the Recorder commits. This is independent of SwiftUI re-render cycles, so Shun's conflict resolution and notification posting now run every time.

Downstream effects (unchanged, now actually running):

- `resolveConflicts()` clears the old owner's combo via `KeyboardShortcuts.reset(...)` and sets the 3-second "Reassigned from X" caption on the new row.
- `.shortcutsDidChange` triggers `ShortcutManager.refreshRegistrations()`, which rebuilds the CGEvent tap's lookup table with the new combo mapping.

### Add logging on the conflict path

`ShortcutManager` already logs extensively (`os.Logger` subsystem `com.shun`, category `ShortcutManager`). The UI conflict path has no logging, which is exactly why this bug survived until a user reported it. Add a single log line inside `resolveConflicts` when a reassignment actually fires:

```swift
// Sources/Shun/Views/ShortcutListView.swift — inside ShortcutRow
private static let log = Logger(subsystem: "com.shun", category: "ShortcutRow")

private func resolveConflicts(newShortcut: KeyboardShortcuts.Shortcut?) {
    guard let newShortcut else {
        conflictNotice = nil
        return
    }
    for other in store.shortcuts where other.id != shortcut.id {
        if let otherCombo = KeyboardShortcuts.getShortcut(for: other.keyboardShortcutName),
           otherCombo == newShortcut
        {
            Self.log.info("reassigned \(newShortcut) from \(other.displayName, privacy: .public) to \(shortcut.displayName, privacy: .public)")
            KeyboardShortcuts.reset(other.keyboardShortcutName)
            conflictNotice = "Reassigned from \(other.displayName)"
            // ... existing clear-after-3s
            return
        }
    }
    conflictNotice = nil
}
```

Cheap, one line at the hot moment, prints in Console.app. Next time someone reports "nothing happens", we can filter by subsystem and see immediately whether the callback fired and what was reassigned.

## Scope

- Single file: `Sources/Shun/Views/ShortcutListView.swift`.
- Single function: `ShortcutRow.recorder` computed property.
- No changes to `ShortcutManager`, `ShortcutStore`, `AppShortcut`, or any other view.

## Non-goals

Out of scope for this change (deliberately, per KISS / YAGNI):

- System/OS-reserved combo detection (Cmd+Space, Cmd+Tab, etc.).
- Generalised "press does nothing" diagnostic UI (accessibility permission, globalEnabled off, etc.).
- Recipe-import conflict per-entry reporting.
- Confirmation dialog before stealing (current auto-steal UX is acceptable per user).
- Mirroring the "Reassigned from X" caption onto the blanked row (possible follow-up).

## Testing

Manual:

1. Launch Shun. Add two apps (e.g. Firefox, Chrome).
2. Assign ⌘⇧A to Firefox via its Recorder.
3. Click Firefox away from Shun (Finder, etc.), press ⌘⇧A — Firefox should activate. Confirms baseline works.
4. Return to Shun. On Chrome's row, record ⌘⇧A.
5. Expected: Firefox's recorder goes blank. Chrome's recorder shows ⌘⇧A. Orange caption "Reassigned from Firefox" under Chrome's recorder for ~3s.
6. Click away from Shun, press ⌘⇧A — Chrome should activate.
7. Re-record ⌘⇧A back onto Firefox — same symmetric behaviour.

Regression checks:

- Recording a fresh combo on a row that had none still works and registers.
- Clearing a combo via Delete key still works and unregisters (verify pressing the combo afterwards does nothing).
- Global toggle off → no combo fires. Toggle on → fires again.

### Verifying the fix actually runs (not just the end-user symptom)

The end-user test ("press the combo, right app activates") can pass even if `resolveConflicts` silently missed a branch, as long as `refreshRegistrations` happens to rebuild the tap table via some other path. To prove the callback fires, add two in-loop checks:

1. **Immediate UI response**: in step 5 of the manual test, the old owner's recorder must go empty *immediately* (before clicking away, before any other interaction). If the old combo is still visually shown, the `onChange` did not fire — or fired but the row's NSView didn't refresh (the second is documented as a known follow-up; see §3 of the design review).
2. **Log evidence**: with Console.app filtered by `subsystem:com.shun category:ShortcutRow`, recording a conflicting combo must print the `reassigned … from X to Y` line. No line = callback didn't fire = fix didn't land.

If either of these fails, the fix is not working regardless of what pressing the hotkey appears to do.

## Follow-ups (not in this change)

- Consider showing the "Reassigned" notice on the blanked row too (requires lifting conflict state to the parent `ShortcutListView`).
- Consider a clearer affordance (e.g. confirmation sheet) if users report accidental overwrites.

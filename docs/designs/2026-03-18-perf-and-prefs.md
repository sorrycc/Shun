# HotApp: Performance + Preferences Improvements

**Date**: 2026-03-18
**Approach**: Surgical fixes — no new files, no new abstractions

## Performance

### 1. Remove `fileExists` from AppShortcut decode/init

**File**: `Sources/HotApp/Models/AppShortcut.swift`

**Problem**: `FileManager.default.fileExists(atPath:)` is called during JSON decoding (`init(from decoder:)` line 47) and during `init(...)` (line 30) for every shortcut. This couples deserialization with I/O and is redundant — `ShortcutStore.refreshStaleness()` already recomputes staleness after load.

**Change**:
- In `init(...)`: remove `self.isStale = !FileManager.default.fileExists(atPath: bundleURL.path)`, replace with `self.isStale = false`
- In `init(from decoder:)`: same — default `isStale = false`
- Ensure `ShortcutStore.load()` calls `refreshStaleness()` after loading (it already does via existing flow)

### 2. Async AppScanner + shared instance + icon cache

**File**: `Sources/HotApp/Services/AppScanner.swift`

**Problem**:
- `scan()` is synchronous on `@MainActor` — directory enumeration, `Bundle(url:)` init, Info.plist reads, and `NSWorkspace.shared.icon(forFile:)` all block the main thread
- Each `AppSearchView` creates a new `AppScanner` instance, so the 60s cache is never reused across sheet opens

**Changes**:
- Add `static let shared = AppScanner()` for cache persistence across sheet opens
- Add `private var iconCache: [String: NSImage] = [:]` keyed by bundle path
- **Concurrency approach**: Remove `@MainActor` from `AppScanner`. Make it a plain `final class` (or use an actor). Directory enumeration + Bundle reads run on the caller's context (off main thread when called from `async` context). Batch the icon fetches on `@MainActor` afterward since `NSWorkspace.shared.icon(forFile:)` requires it. The public API becomes `func scanAsync() async -> [ScannedApp]`.
- Keep synchronous `scan()` as fallback if needed
- **Cache eviction**: Call `invalidateCache()` when the settings window dismisses to free memory. The 60s TTL handles normal staleness; explicit eviction handles the "window closed, user won't need icons for a while" case.

### 3. Cache icons in ShortcutRow

**File**: `Sources/HotApp/Views/ShortcutListView.swift` (ShortcutRow, line 151)

**Problem**: `NSWorkspace.shared.icon(forFile:)` is called inside `body` on every SwiftUI re-evaluation. Each scroll, toggle, or state change refetches icons from disk for all visible rows.

**Change**:
- Add `private static var iconCache: [String: NSImage] = [:]` on `ShortcutRow`
- In `appIcon`, check cache first; only call `NSWorkspace.shared.icon(forFile:)` on cache miss
- Alternatively, reuse `AppScanner.shared`'s icon cache if the scanner is already shared

### 4. Targeted shortcut registration

**File**: `Sources/HotApp/Services/ShortcutManager.swift`

**Problem**: `refreshRegistrations()` unregisters ALL then re-registers ALL shortcuts on every single CRUD operation. O(n) teardown + O(n) setup for every change.

**Changes**:
- Keep `refreshRegistrations()` for bulk operations (app launch, global toggle)
- Add `func registerSingle(_ shortcut: AppShortcut)` — registers one shortcut
- Add `func unregisterSingle(_ shortcut: AppShortcut)` — unregisters one shortcut

**File**: `Sources/HotApp/Views/ShortcutListView.swift`, `Sources/HotApp/Views/AppSearchView.swift`

**Change**: For add/remove/toggle operations, call targeted methods instead of posting `.shortcutsDidChange`. The notification is only posted for global state changes (global toggle, reorder).

**File**: `Sources/HotApp/AppDelegate.swift`

**Change**: Expose `shortcutManager` via `ShortcutStore`. Give `ShortcutStore` a weak reference to `ShortcutManager` and add convenience methods like `store.registerAfterAdd(_:)` / `store.unregisterBeforeRemove(_:)`. This keeps views simple — they already talk to `store`, no second environment object needed.

### 5. Smarter refreshStaleness

**File**: `Sources/HotApp/AppDelegate.swift` (lines 36-57)

**Problem**: Every time *any* app launches or quits, `refreshStaleness()` iterates all shortcuts and does `fileExists` for each.

**Changes**:
- Extract `bundleIdentifier` from notification's `userInfo` (`NSRunningApplication`)
- Only check staleness for shortcuts matching that bundleIdentifier

**File**: `Sources/HotApp/Models/ShortcutStore.swift`

**Change**: Add `func refreshStaleness(for bundleIdentifier: String)` that only checks matching shortcut(s). Keep the full `refreshStaleness()` for initial load.

### 6. Debounced save

**File**: `Sources/HotApp/Models/ShortcutStore.swift`

**Problem**: `save()` is called synchronously on every mutation — add, remove, move, toggle. Dragging a shortcut through 5 list positions fires 5 JSON encode + atomic file writes.

**Change**:
- Add a `private var saveTask: Task<Void, Never>?` property
- In `save()`, cancel the previous `saveTask` and schedule a new one with a 0.3s delay (`Task.sleep(for: .milliseconds(300))`)
- Add `func saveImmediately()` for cases that need guaranteed persistence (e.g., `remove()` — don't want to lose a deletion if the app crashes)
- `add()` and `remove()` call `saveImmediately()`; `move()` and `toggleEnabled()` use debounced `save()`

### 7. Refresh staleness on settings window open

**File**: `Sources/HotApp/Views/SettingsView.swift` or `Sources/HotApp/HotAppApp.swift`

**Problem**: Staleness is only refreshed on workspace launch/terminate notifications. If an app is *deleted* (not terminated) while HotApp is running, staleness is never updated until relaunch.

**Change**:
- Call `store.refreshStaleness()` in an `.onAppear` modifier on `SettingsView` or `ShortcutListView`
- Lightweight — only iterates shortcuts and calls `fileExists` when the user actually opens the window

## Preferences UI

### 8. Multi-add in AppSearchView

**File**: `Sources/HotApp/Views/AppSearchView.swift`

**Problem**: `dismiss()` is called after adding one app (line 112), forcing the user to reopen the sheet to add more.

**Changes**:
- Remove `dismiss()` from `addApp()`
- Add a "Done" button in the search field bar or as a toolbar item to close the sheet
- Use `AppScanner.shared` instead of `private let scanner = AppScanner()`
- Already-added apps continue to show the "Added" badge (existing behavior)

### 9. Shortcut conflict detection (last-write-wins)

**File**: `Sources/HotApp/Views/ShortcutListView.swift` (ShortcutRow.recorder)

**Problem**: Two apps can be assigned the same hotkey silently. The second registration overwrites the first with no feedback.

**Resolution strategy**: Last-write-wins (matches Thor's behavior). When a conflict is detected, auto-clear the previous binding and show a brief notice.

**Change**:
- In the recorder's `onChange`, iterate `store.shortcuts` and call `KeyboardShortcuts.getShortcut(for:)` for each, comparing against the just-assigned combo
- If conflict found: call `KeyboardShortcuts.reset(conflicting.keyboardShortcutName)` to clear the old binding
- Show a temporary notice (e.g., inline text below the recorder or a brief overlay): "Shortcut reassigned from {OtherAppName}"
- No blocking dialogs — keep the flow smooth

### 10. Delete confirmation

**File**: `Sources/HotApp/Views/ShortcutListView.swift`

**Problem**: Removing a shortcut is instant with no undo or confirmation.

**Change**:
- Add `@State private var showDeleteConfirmation = false` and `@State private var pendingDeleteId: UUID?`
- Wrap `removeSelected()` and `onDelete` to set these states instead of deleting directly
- Show `.confirmationDialog("Remove shortcut for {appName}?")` with destructive "Remove" action

## Design decisions

- **Concurrency**: Remove `@MainActor` from `AppScanner`. Scan off main thread, batch icon fetches on MainActor after.
- **Manager access**: Views access `ShortcutManager` through `ShortcutStore` (weak ref + convenience methods), not a second environment object.
- **Conflict resolution**: Last-write-wins with notice, matching Thor. No blocking dialogs.
- **Save strategy**: Debounced (0.3s) for reorder/toggle, immediate for add/remove.
- **Icon cache lifecycle**: Cleared on settings window dismiss via `invalidateCache()`.

## Summary of files changed

| File | Changes |
|---|---|
| `AppShortcut.swift` | Remove `fileExists` from init/decode |
| `AppScanner.swift` | Remove `@MainActor`, `static shared`, `scanAsync()`, icon cache + eviction |
| `ShortcutManager.swift` | Add `registerSingle` / `unregisterSingle` |
| `ShortcutStore.swift` | Weak ref to manager, `refreshStaleness(for:)`, debounced `save()` |
| `AppDelegate.swift` | Filter workspace notifications by bundleID, wire store↔manager |
| `AppSearchView.swift` | Multi-add, use shared scanner |
| `ShortcutListView.swift` | Icon cache, conflict detection (last-write-wins), delete confirmation |
| `SettingsView.swift` | `onAppear` → `refreshStaleness()` |

No new files. No new dependencies. 10 changes across 8 files.

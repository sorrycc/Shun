# Usage Insights & Smart Suggestions — Design Document

**Date:** 2026-03-18
**Status:** Approved
**Summary:** Track per-shortcut invocation counts, show inline usage badges in the shortcut list, and add an Insights tab to Settings.

---

## Decisions

| Decision | Choice |
|---|---|
| Log timing | Async dispatch to main thread after toggle action |
| UI placement | Inline badges on ShortcutListView + new Insights tab |
| Tracking scope | HotApp activations only (no system-wide tracking) |
| Time window | Rolling 7 days, prune on load and on save |
| Storage | JSON log file (`usage.json`) alongside `shortcuts.json` |

---

## Data Model

### UsageEvent

```swift
// New file: Models/UsageEvent.swift
struct UsageEvent: Codable, Sendable {
    let bundleIdentifier: String
    let timestamp: Date
}
```

Flat array stored in `~/Library/Application Support/HotApp/usage.json`. Entries older than 7 days are pruned on load and on each save (cheap date filter), so the array stays bounded even if the app runs for weeks without restart.

### UsageStore

```swift
// New file: Models/UsageStore.swift
@Observable @MainActor
final class UsageStore {
    private(set) var events: [UsageEvent] = []
    private let persistence: UsagePersistence

    func log(bundleIdentifier: String)                      // append + debounced save
    func load()                                              // load + prune >7 days
    func count(for bundleId: String) -> Int                  // this week's count
    func topApps(limit: Int) -> [(bundleId: String, count: Int)]
}
```

Owned by `AppDelegate` alongside `ShortcutStore`. Injected into SwiftUI via `.environment()`.

---

## Logging Hook

In `ShortcutManager`, after `_executeToggle()` fires, dispatch to main:

```swift
DispatchQueue.main.async {
    usageStore.log(bundleIdentifier: entry.bundleIdentifier)
}
```

Implementation: add a `logActivation: @Sendable (String) -> Void` closure to `TapContext`. `ShortcutManager.init` gains a `usageStore: UsageStore` parameter. The closure is set during init and dispatches to main to call `UsageStore.log()`.

**Both activation paths must log:**
- **CGEvent tap path:** call `logActivation` from `_executeToggle()` for `.activate` and `.launch` cases (not `.hide` — hiding is not "using" the shortcut to switch to an app)
- **Carbon fallback path:** call `usageStore.log()` inside each `KeyboardShortcuts.onKeyUp` closure in `registerCarbon()`

### Save Debouncing

`UsageStore.log()` appends to the in-memory array but does NOT write to disk immediately. Saves are debounced with a 300ms coalesce (same pattern as `ShortcutStore.save()`). A `saveImmediately()` is called on `applicationWillTerminate` to flush any pending events.

> **Note:** `AppDelegate` does not currently implement `applicationWillTerminate(_:)`. This method must be added to call `usageStore.saveImmediately()`.

---

## Persistence

```swift
// Add to PersistenceService.swift or separate file
struct UsagePersistence: Sendable {
    // File: ~/Library/Application Support/HotApp/usage.json
    func load() throws -> [UsageEvent]
    func save(_ events: [UsageEvent]) throws
}
```

Follows the same pattern as `JSONFilePersistence`.

---

## UI

### Inline Badges (ShortcutListView)

In `ShortcutRow`, below the app name in the existing `VStack(alignment: .leading)`:

```
[App Icon] Terminal          [recorder]    [toggle]
           12x past 7 days
```

- Read from `UsageStore.count(for: shortcut.bundleIdentifier)`
- Only shown if count > 0
- `Text("\(count)× past 7 days").font(.caption2).foregroundStyle(.secondary)`
- Label says "past 7 days" (not "this week") since the window is a rolling 7-day period, not a calendar week

### Insights Tab

New third tab in `SettingsView`:

```
[Shortcuts] [General] [Insights]
```

**InsightsView** contents (new file: `Views/InsightsView.swift`):
- **Empty state:** When no usage data exists yet, show: "Start using your shortcuts — insights will appear here."
- **Header stats:** "You've used shortcuts **142 times** in the past 7 days"
- **Top apps list:** Ranked by activation count — app icon + name + count + horizontal bar
- **Clear data:** "Clear Usage Data" button at the bottom with a confirmation dialog. Deletes `usage.json` and resets in-memory state.
- Layout: `Form` with `Section`s, consistent with `GeneralSettingsView`

> **Dropped: "Least used" section.** Showing "consider reassigning?" for 0-activation shortcuts is noisy — the user may have just added them. Would require tracking shortcut creation dates to avoid false positives. Not worth the complexity in v1; revisit if users request it.
>
> **Dropped: AppSearchView sorting by usage.** Since we only track HotApp activations, apps with usage data are exactly the apps that already have shortcuts. Those are already shown dimmed with a checkmark in AppSearchView. Sorting them to the top adds no value.

---

## Files to Create / Modify

**New files:**
- `Sources/HotApp/Models/UsageEvent.swift`
- `Sources/HotApp/Models/UsageStore.swift`
- `Sources/HotApp/Views/InsightsView.swift`

**Modified files:**
- `Sources/HotApp/Services/PersistenceService.swift` — add `UsagePersistence`
- `Sources/HotApp/Services/ShortcutManager.swift` — add `usageStore` init parameter, add logging closure to `TapContext`, fire on activation in both CGEvent tap and Carbon paths
- `Sources/HotApp/AppDelegate.swift` — create `UsageStore`, pass to `ShortcutManager.init`, inject via `.environment()`, add `applicationWillTerminate` to flush pending saves
- `Sources/HotApp/HotAppApp.swift` — pass `usageStore` environment to window
- `Sources/HotApp/Views/SettingsView.swift` — add Insights tab
- `Sources/HotApp/Views/ShortcutListView.swift` — add inline usage badges in `ShortcutRow`

---

## Out of Scope

- System-wide app tracking (only HotApp activations are logged)
- Time-of-day heatmaps (data supports it but no UI in v1)
- "Add hotkey for apps you use often" suggestions (requires system-wide tracking)
- "Least used shortcuts" section (noisy without creation-date tracking)
- AppSearchView usage-based sorting (redundant — tracked apps already have shortcuts)

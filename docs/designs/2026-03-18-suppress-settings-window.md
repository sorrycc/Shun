# Suppress Settings Window on Launch

## Problem

Three issues with current behavior:
1. Settings window auto-shows on every app launch (SwiftUI `Window` scene default)
2. App quits when the settings window is closed (no `applicationShouldTerminateAfterLastWindowClosed` override)
3. First-launch auto-open should be preserved

## Approach

Use `.defaultLaunchBehavior(.suppressed)` (macOS 15+ API) to prevent SwiftUI from auto-showing the settings window on launch. Add `applicationShouldTerminateAfterLastWindowClosed` returning `false` to keep the app alive in the menu bar.

## Changes

### `Package.swift`

Bump deployment target from macOS 14 to macOS 15 (required for `.defaultLaunchBehavior`):

```swift
platforms: [
    .macOS(.v15),
]
```

### `Sources/HotApp/HotAppApp.swift`

Add `.defaultLaunchBehavior(.suppressed)` to the `Window` scene:

```swift
Window("HotApp Settings", id: "settings") {
    SettingsView()
        .environment(appDelegate.store)
        .onReceive(NotificationCenter.default.publisher(for: .openSettings)) { _ in
            openWindow(id: "settings")
        }
}
.defaultSize(width: 520, height: 400)
.windowResizability(.contentSize)
.defaultLaunchBehavior(.suppressed)  // <-- add
```

### `Sources/HotApp/AppDelegate.swift`

Add to the `AppDelegate` class:

```swift
func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
    false
}
```

No change to the `isFirstLaunch` block -- it explicitly calls `openSettings()` which posts `.openSettings` notification, triggering `openWindow(id: "settings")`, so first-launch behavior is preserved.

## Notes

**First-launch timing coupling**: The `isFirstLaunch` block in `AppDelegate` uses a 500ms delay before posting `.openSettings`. This delay exists to ensure the SwiftUI `Window` scene is initialized and its `.onReceive` handler is ready to respond. With `.defaultLaunchBehavior(.suppressed)`, the window is not shown but the scene is still initialized, so the 500ms delay continues to work. If this delay is ever removed or shortened, the first-launch settings window may fail to appear.

## Behavior

| Scenario | Before | After |
|---|---|---|
| Normal launch | Settings window appears | Menu bar icon only |
| First launch | Settings window appears | Settings window appears (unchanged) |
| Close settings window | App quits | Window closes, app stays in menu bar |
| Click "Settings" in menu | Opens settings | Opens settings (unchanged) |

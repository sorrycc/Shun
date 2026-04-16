# Shun — Claude Code Project Guide

## What is this?
macOS 14+ menu bar app for global hotkey-to-app switching (Thor clone). SPM-only, Swift 6 strict concurrency, hybrid SwiftUI + AppKit.

> Formerly named **HotApp** — renamed to **Shun** (瞬, "instant"). Legacy design docs under `docs/` still use the old name.

## Build & Run
```bash
swift build                      # debug build
swift build -c release           # release build
./Scripts/package_app.sh         # create .app bundle (release + codesign)
```

## Architecture (key files)
- `Sources/Shun/ShunApp.swift` — @main App, Window scene (NOT Settings scene — see caveat below)
- `Sources/Shun/AppDelegate.swift` — owns ShortcutStore + ShortcutManager, NSStatusItem lifecycle
- `Sources/Shun/StatusItemController.swift` — AppKit NSMenu, notification names (.shortcutsDidChange, .openSettings)
- `Sources/Shun/Models/AppShortcut.swift` — plain Codable struct, isStale excluded from JSON via CodingKeys
- `Sources/Shun/Models/ShortcutStore.swift` — @Observable @MainActor, CRUD + persistence
- `Sources/Shun/Services/ShortcutManager.swift` — registers global hotkeys via KeyboardShortcuts, toggle logic
- `Sources/Shun/Services/AppScanner.swift` — scans /Applications for .app bundles, 60s cache
- `Sources/Shun/Services/PersistenceService.swift` — protocol + JSONFilePersistence, one-time migration from legacy HotApp support dir
- `Sources/Shun/Views/` — SwiftUI views: SettingsView (tabs), ShortcutListView, AppSearchView

## Critical Conventions
- **Swift 6 strict concurrency**: all UI types are `@MainActor`. `AppShortcut` is `Sendable`.
- **LSUIElement app** (no Dock icon): must use `Window` scene, NOT `Settings` scene (it silently fails for LSUIElement apps)
- **Settings window bridge**: AppDelegate posts `.openSettings` notification → ShunApp listens with `.onReceive()` → calls `openWindow(id: "settings")`
- **KeyboardShortcuts dynamic names**: each AppShortcut has a `shortcutName` ("hotapp_{uuid}") that maps to `KeyboardShortcuts.Name`. Prefix kept as `hotapp_` for backward compatibility with existing user data — do not change it without a data migration. Key combos stored in UserDefaults by KeyboardShortcuts, NOT in our JSON.
- **Dual persistence**: shortcuts.json in ~/Library/Application Support/Shun/ + UserDefaults for key combos and globalEnabled. On first launch after rename, PersistenceService moves the legacy `HotApp/` dir to `Shun/`.
- **Orphan cleanup**: `cleanupOrphanedDefaults()` removes stale KeyboardShortcuts_ UserDefaults entries on load
- **Notification-driven refresh**: CRUD operations post `.shortcutsDidChange` → AppDelegate calls `shortcutManager.refreshRegistrations()`

## Dependencies
- `KeyboardShortcuts` (sindresorhus, v1.15.0) — global hotkey registration + SwiftUI Recorder view

## Design Doc
Full spec: `docs/designs/2026-03-18-hotapp.md` (uses the old name)

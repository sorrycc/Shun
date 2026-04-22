# Check for Updates — Activate App Before Showing Sparkle Window

## 1. Background
Shun is an LSUIElement (no Dock icon) menu-bar app. When the user picks "Check for Updates…" from the status bar menu, the Sparkle update window appears but is not frontmost/key — the user must click it to focus before interacting. The same issue was already solved for the Settings window by calling `NSApp.activate()` before posting `.openSettings`.

## 2. Requirements Summary
**Goal:** Clicking "Check for Updates…" in the status bar menu should bring Shun to the foreground and present the Sparkle update window as the active, key, frontmost window.

**Scope:**
- In: The `.checkForUpdates` notification path in `AppDelegate.observeUpdateChecks`.
- Out: Sparkle's automatic background update checks (remain unobtrusive); `openSettings()` (already correct); `UpdaterService` API; `StatusItemController`.

**Key Decisions:** Mirror the existing `openSettings()` pattern (call `NSApp.activate()` before dispatching the action). Fix at the AppDelegate observer, not the menu handler, to keep the notification-driven pattern intact.

## 3. Acceptance Criteria
1. Clicking "Check for Updates…" from the menu brings Shun to the foreground before Sparkle's window appears.
2. The Sparkle update window is key and frontmost (accepts keyboard input immediately, no extra click needed).
3. Works whether Shun is hidden, backgrounded, or another app is frontmost.
4. No regression in `openSettings()` behavior or automatic background update checks.
5. Build passes under Swift 6 strict concurrency with no new warnings.

## 4. Problem Analysis
Current state (`AppDelegate.swift:126-136`):

```swift
private func observeUpdateChecks() {
    NotificationCenter.default.addObserver(
        forName: .checkForUpdates,
        object: nil,
        queue: .main
    ) { [weak self] _ in
        Task { @MainActor in
            self?.updater?.checkForUpdates()
        }
    }
}
```

The observer triggers Sparkle without first activating the app. Because the app is LSUIElement, macOS does not auto-activate it when a window is created — Sparkle's window appears but is not key.

Compare to `openSettings()` (`AppDelegate.swift:56-59`), which already handles this:

```swift
private func openSettings() {
    NSApp.activate()
    NotificationCenter.default.post(name: .openSettings, object: nil)
}
```

- **Approach A** — Call `NSApp.activate()` inside `StatusItemController.checkForUpdates` before posting the notification -> rejected: spreads UI activation concerns into the menu layer; inconsistent with `openSettings` (activation lives in AppDelegate there — actually it's in AppDelegate's `openSettings`, called via closure from StatusItemController).
- **Approach B** — Call `NSApp.activate()` inside `UpdaterService.checkForUpdates` -> rejected: leaks UI concerns into a service wrapper; would also activate on any caller (including future programmatic callers) whether they want to or not.
- **Chosen approach** — Call `NSApp.activate()` inside the AppDelegate observer before invoking `updater.checkForUpdates()`. Keeps the pattern consistent with `openSettings()`, minimal blast radius.

## 5. Decision Log

**1. Where to call `NSApp.activate()`?**
- Options: A) `StatusItemController.checkForUpdates` menu handler · B) `AppDelegate.observeUpdateChecks` observer · C) `UpdaterService.checkForUpdates`
- Decision: **B)** — Mirrors `openSettings()` at `AppDelegate.swift:56-59`, keeps notification-driven pattern, single place responsible for activation.

**2. Which activation API?**
- Options: A) `NSApp.activate()` (macOS 14+) · B) `NSApp.activate(ignoringOtherApps: true)` (deprecated on 14+)
- Decision: **A)** — `openSettings()` already uses this. Project targets macOS 14+.

**3. Order: activate then trigger, or trigger then activate?**
- Options: A) Activate first · B) Trigger first
- Decision: **A)** — Ensures the app is frontmost when Sparkle creates its window, so it becomes key immediately. Matches `openSettings()`.

## 6. Design
Two-line change to the observer body. Before invoking Sparkle, call `NSApp.activate()` on the main actor. The menu item, notification, and UpdaterService are unchanged.

```swift
private func observeUpdateChecks() {
    NotificationCenter.default.addObserver(
        forName: .checkForUpdates,
        object: nil,
        queue: .main
    ) { [weak self] _ in
        Task { @MainActor in
            NSApp.activate()
            self?.updater?.checkForUpdates()
        }
    }
}
```

Notes:
- `updater` may be nil in dev (`swift run`) where Sparkle is skipped — `NSApp.activate()` is still safe and harmless there.
- Observer already dispatches onto `@MainActor`, so `NSApp.activate()` runs on the main thread.

## 7. Files Changed
- `Sources/Shun/AppDelegate.swift` — add `NSApp.activate()` before `updater?.checkForUpdates()` inside the `.checkForUpdates` observer.

## 8. Verification
1. [AC1, AC2, AC3] Package a build (`./Scripts/package_app.sh`), launch Shun, switch focus to another app (e.g., Safari), then click the Shun menu-bar icon → "Check for Updates…". The Sparkle window should appear as the frontmost, key window; keystrokes (e.g., Esc/Return) should act on it immediately with no extra click.
2. [AC1] Repeat while Shun has no open windows — still activates and shows the Sparkle window frontmost.
3. [AC4] Open Settings from the menu — still works unchanged. Wait for a scheduled background check (or observe existing behavior) — no new foreground-stealing occurs because only the manual menu path triggers activation.
4. [AC5] `swift build -c release` succeeds with no new warnings.

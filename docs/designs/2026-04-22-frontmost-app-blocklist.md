# Frontmost App Blocklist — pass hotkeys through when certain apps are active

## 1. Background

Shun registers global hotkeys that intercept keystrokes via a CGEvent tap. When the user is running an app like **Screen Sharing.app** (remotely controlling another Mac), they want the hotkey to reach the remote Mac — not trigger Shun's local toggle. Today, Shun consumes the event regardless of which app is frontmost (except its own, where `paused` is set so the in-app Recorder can capture keys).

We need a narrow, hardcoded escape hatch for this case. User scope: "hard code with it for now and might make it configurable later."

## 2. Requirements Summary

**Goal:** When a blocklisted app (initially `com.apple.ScreenSharing`) is frontmost, Shun must not intercept global hotkeys — let keystrokes pass through to the frontmost app.

**Scope:**
- In scope: Pass-through in CGEvent tap hot path; equivalent behavior in the Carbon fallback; hardcoded single-entry blocklist; correct state at launch.
- Out of scope: User-facing configuration UI, persistence, menubar indicator, prefix/wildcard matching, multi-entry list, migration.

## 3. Acceptance Criteria

1. **Pass-through while active** — With a global hotkey registered for any app, activating `Screen Sharing.app` and pressing that hotkey does NOT trigger Shun's toggle; the keystroke is received by Screen Sharing (and forwarded to the remote Mac).
2. **Restores after focus change** — After leaving Screen Sharing (another app becomes frontmost), the same hotkey resumes working normally.
3. **Correct at launch** — Launching Shun while Screen Sharing is already frontmost results in hotkeys being passed through from the first press (no race).
4. **Self-pause preserved** — Opening Shun Settings while Screen Sharing is not frontmost still pauses hotkeys (existing Recorder behavior). Closing Settings resumes them.
5. **Flags independent** — The `paused` (self-active) and blocklist flags never clobber each other.
6. **Carbon fallback consistent** — The Carbon fallback path (used if CGEvent tap creation fails) does not toggle when Screen Sharing is frontmost. (Note: on this path the keystroke has already been consumed at the Carbon HotKey layer before our closure runs, so it does *not* reach Screen Sharing — see §6.4 for why this is acceptable as a last-resort path.)

## 4. Problem Analysis

Current state (`Sources/Shun/Services/ShortcutManager.swift`):

- A CGEvent tap callback (`_makeEventTap`) routes every key-down through `TapContext.matchAndResolve(...)`.
- `TapContext` holds a `paused` bool, set via `NSApplication.didBecomeActive/ResignActive` observers (so the in-app KeyboardShortcuts Recorder can capture keystrokes when Shun itself is active).
- The frontmost app is already tracked in `TapContext._frontmostApp`, updated by `NSWorkspace.didActivateApplicationNotification`. It's used to decide hide-vs-activate.
- A Carbon fallback path (`registerCarbon`) runs on main thread via a `KeyboardShortcuts.onKeyUp` closure.

Approaches evaluated:

- **A — Check `NSWorkspace.shared.frontmostApplication` inside `matchAndResolve` on every keystroke.** Rejected: the CGEvent tap callback runs on a dedicated high-QoS thread (`Shun.EventTap`), and `NSWorkspace` / `NSRunningApplication` are not documented as safe to access off main. The existing design deliberately *caches* the frontmost app on main and reads only our own lock-guarded state from the tap thread. Option A would violate that invariant.
- **B — Overload the existing `paused` flag.** Rejected: `paused` semantically means "Shun is frontmost (Recorder needs events)". Two distinct conditions driving one bit risks bugs when both sources mutate it (the resign-active observer would incorrectly clear a blocklist-set pause).
- **Chosen — Add a second independent flag `_blockedByFrontmost` on `TapContext`.** Updated by the existing `didActivateApplicationNotification` observer + seeded at launch. Hot-path guard becomes `!paused && !blockedByFrontmost`.

## 5. Decision Log

**1. How to check frontmost vs. blocklist?**
- Options: A) Inline in `matchAndResolve` each keystroke · B) Maintain a bool updated on focus change
- Decision: **B)** — focus changes are rare, keystrokes are hot-path. We already observe activations.

**2. Reuse `paused` or add a new flag?**
- Options: A) Overload `paused` · B) New `_blockedByFrontmost`
- Decision: **B)** — orthogonal semantics; two different observers; overloading risks one clobbering the other.

**3. Where does the blocklist constant live?**
- Options: A) File-private in `ShortcutManager.swift` · B) New `Config/Blocklist.swift` · C) Inside `TapContext`
- Decision: **A)** — one string, one file, one usage. CLAUDE.md warns against premature abstraction. Extract when configurable.

**4. Handle Carbon fallback?**
- Options: A) Skip (rare path) · B) Inline `frontmostApplication` check in closure
- Decision: **B)** — trivial; keeps behavior consistent. Closure already runs on main.

**5. Seed at launch?**
- Options: A) Wait for first activation · B) Compute `_blockedByFrontmost` during `seedRunningAppCache`
- Decision: **B)** — AC3 requires first-keystroke correctness.

**6. Bundle ID matching?**
- Options: A) Exact (`==`) · B) Prefix
- Decision: **A)** — no known helper variants of `com.apple.ScreenSharing` need suppression; prefix risks false positives.

**7. Menubar indicator?**
- Options: A) Show "paused because Screen Sharing is active" · B) Silent
- Decision: **B)** — transient, user-understood, scope creep. Log at `.info` for debuggability.

## 6. Design

### 6.1 Data model

Add a file-private constant at **file scope** near the top of `ShortcutManager.swift` (alongside the other `private let` helpers like `modifierOnlyMask` — **not** inside `TapContext`, so the Carbon closure in `ShortcutManager` can read it without plumbing):

```swift
/// Bundle IDs whose frontmost presence suppresses Shun's hotkey interception.
/// Keystrokes pass through to the app instead of triggering our toggle.
/// Hardcoded today; likely becomes user-configurable later.
private let blockedFrontmostBundleIDs: Set<String> = [
    "com.apple.ScreenSharing",
]
```

### 6.2 `TapContext` changes

Add a thread-safe `_blockedByFrontmost` bool, protected by the same `OSAllocatedUnfairLock`:

```swift
private var _blockedByFrontmost = false
```

Change `updateFrontmost(_:)` to also recompute the flag. Snapshot the previous value inside the lock, then log *after* releasing it — the same unfair lock is contended by `matchAndResolve` on every keystroke, so no I/O should happen while holding it:

```swift
func updateFrontmost(_ app: NSRunningApplication?) {
    // Snapshot transition inside the lock; log after releasing.
    let (didEngage, newBundleId): (Bool?, String?) = lock.withLock {
        let wasBlocked = _blockedByFrontmost
        _frontmostApp = app
        let bundleId = app?.bundleIdentifier
        _blockedByFrontmost = bundleId.map(blockedFrontmostBundleIDs.contains) ?? false
        let transition: Bool? = (wasBlocked == _blockedByFrontmost) ? nil : _blockedByFrontmost
        return (transition, bundleId)
    }
    if let engaged = didEngage {
        log.info("frontmost blocklist \(engaged ? "engaged" : "released"): \(newBundleId ?? "nil", privacy: .public)")
    }
}
```

Extend the top of `matchAndResolve(keyCode:flags:)`. The guard lives **inside the existing `lock.withLock { ... }`** — we read `_blockedByFrontmost` directly (same lock is already held), not via a computed accessor that would re-enter the lock:

```swift
func matchAndResolve(keyCode: Int64, flags: CGEventFlags) -> ToggleAction? {
    lock.withLock {
        guard !_paused, !_blockedByFrontmost else { return nil }
        // ...existing match logic unchanged...
    }
}
```

The `return nil` from `matchAndResolve` causes the outer tap callback to fall through to `return Unmanaged.passUnretained(event)` — the event is NOT consumed. This is what we want.

### 6.3 Seed correctness

In `ShortcutManager.seedRunningAppCache()`, the existing call `context.updateFrontmost(NSWorkspace.shared.frontmostApplication)` already runs through the updated setter, so `_blockedByFrontmost` is correctly initialized. No additional code needed — the change lives in `updateFrontmost`.

### 6.4 Carbon fallback

Inside `registerCarbon`'s `KeyboardShortcuts.onKeyUp` closure, add an early return if the frontmost app is blocklisted. Placed at the very top so it suppresses both the hide branch and the activate/launch branch — even if a user ever binds a shortcut to a blocklisted app itself, we skip the toggle:

```swift
KeyboardShortcuts.onKeyUp(for: name) {
    let workspace = NSWorkspace.shared
    // Pass-through if a blocklisted app is frontmost (e.g., Screen Sharing
    // should keep the keystroke even if the user's shortcut targets it).
    if let frontId = workspace.frontmostApplication?.bundleIdentifier,
       blockedFrontmostBundleIDs.contains(frontId)
    {
        return
    }
    // ...existing toggle logic...
}
```

Note: in the Carbon path, KeyboardShortcuts has already consumed the key at the Carbon HotKey layer, so the event does not reach Screen Sharing. This is an inherent limitation of the Carbon fallback (KeyboardShortcuts itself registers the Carbon hotkey). Our options are (a) don't toggle, event is still consumed — what we're doing, or (b) tear down/re-register shortcuts on focus change — churn and complexity. We choose (a) as "best effort" — this path is only hit when `CGEvent.tapCreate` fails (typically missing Accessibility permission), which is itself an already-broken state. Documented as a known limitation.

### 6.5 Logging

Implemented inline in §6.2's `updateFrontmost` snippet: a single `log.info` fires *only on transition* (engage ↔ release), outside the lock. Useful for debugging on real user systems where the feature might silently pause Shun. Filter:

```bash
log stream --predicate 'subsystem == "com.shun" AND category == "ShortcutManager"' --level info
```

### 6.6 Concurrency

All mutations happen inside the existing `OSAllocatedUnfairLock`. No new threading concerns. `updateFrontmost` is called from the `didActivateApplicationNotification` handler on the main queue and from `seedRunningAppCache` on `@MainActor` init — same code path, already covered by the lock.

### 6.7 Error handling

None required — this is a guard that returns earlier than the existing code. No new failure modes.

## 7. Files Changed

- `Sources/Shun/Services/ShortcutManager.swift` — add `blockedFrontmostBundleIDs` constant; add `_blockedByFrontmost` to `TapContext`; extend `updateFrontmost` to recompute the flag + log transitions; extend `matchAndResolve` guard; add early-return in Carbon fallback closure.

## 8. Verification

Manual verification (project has no test suite):

1. **[AC1] Pass-through while active**
   - Register a hotkey for any app in Shun Settings.
   - Open `Screen Sharing.app` (Applications → Utilities). It can remain on the connection screen — no real remote session needed.
   - With Screen Sharing frontmost, press the hotkey. Expected: Shun does NOT toggle. (Ideally send the key to Screen Sharing's input field to confirm it received the event.)
   - Log stream should show `frontmost blocklist engaged`.

2. **[AC2] Restores after focus change**
   - From the above state, Cmd-Tab to another app. Press the hotkey. Expected: normal Shun toggle behavior.
   - Log should show `frontmost blocklist released`.

3. **[AC3] Correct at launch**
   - Quit Shun. Activate Screen Sharing so it's frontmost. Launch Shun (from terminal: `swift run Shun` or the packaged .app).
   - Without changing focus, press the registered hotkey. Expected: no toggle.

4. **[AC4] Self-pause preserved**
   - With Screen Sharing not running, open Shun Settings. Press the hotkey. Expected: no toggle (self-pause working).
   - Close Settings. Press the hotkey. Expected: toggle fires.

5. **[AC5] Flags independent**
   - Open Shun Settings (self-paused). Activate Screen Sharing (now blocklist-paused). Cmd-Tab back to Shun Settings (self-paused again, blocklist released). Press hotkey in the Recorder field. Expected: Recorder captures the combo (self-pause works even after blocklist round-trip).

6. **[AC6] Carbon fallback**
   - Rarely reachable in practice. If you want to force it: add `return nil` before `CGEvent.tapCreate` in `_makeEventTap` as a local patch. With that, repeat AC1 — Shun should still no-op when Screen Sharing is frontmost.

Log filter:
```bash
log stream --predicate 'subsystem == "com.shun" AND category == "ShortcutManager"' --level info
```

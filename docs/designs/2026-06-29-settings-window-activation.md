# Settings Window Activation Fix (macOS Tahoe)

> Phase 4 review applied: decisions 3–5 and section 6 were revised in response to
> reviewer items B1 (guard state cannot live on the SwiftUI `App` value type),
> B2 (a one-shot `willClose` on `object: nil` reverts prematurely), and N1
> (the `assumeIsolated` justification depends on the delivery queue, not the
> poster). See the `Revised in Phase 4 review` notes inline.

## 1. Background

On macOS 26 (Tahoe), clicking "Settings…" in Shun's menu bar sometimes fails to
show the Settings window. Shun is an `LSUIElement` / `.accessory` app. Two
independent defects combine to produce the intermittent failure:

- **Cause A — Tahoe activation.** `AppDelegate.openSettings()` (`AppDelegate.swift:71`)
  only calls `NSApp.activate()` and never raises the activation policy. macOS
  Tahoe tightened activation for `.accessory` apps, so the app frequently cannot
  become frontmost or let its window take key status — the window opens behind
  the active app or never gains focus.
- **Cause B — subscriber lifecycle.** The `.onReceive(.openSettings) { openWindow(id:"settings") }`
  modifier (`ShunApp.swift:13-14`) is attached to `SettingsView()`, the content
  of the `Window` scene. When that window closes, SwiftUI tears the content (and
  its subscriber) down, so the next menu click posts `.openSettings` into the
  void and nothing reopens the window. Chicken-and-egg: the only thing that opens
  the window must already be inside the open window.

The reference implementation in `~/Projects/Sokki` (same `LSUIElement` shape,
same Sparkle setup) solves both via an app-level observer that flips the
activation policy and fronts the window through raw AppKit.

## 2. Requirements Summary

- **Goal:** the Settings window reliably appears frontmost and key on every
  "Settings…" click, on macOS Tahoe, regardless of prior open/close history.
- **Scope:** `Sources/Shun/ShunApp.swift` and `Sources/Shun/AppDelegate.swift`.
  Also harden the Sparkle "Check for Updates" path, which shares Cause A.
- **Key decisions (approved):** move the `.openSettings` observer to app level;
  present via `NSApp.windows` lookup + `makeKeyAndOrderFront`/`orderFrontRegardless`;
  temporarily flip to `.regular` and revert to `.accessory`; register the revert
  observer once (no leak); KISS — no `ActivationPolicyCoordinator`.

## 3. Acceptance Criteria

1. `swift build` succeeds.
2. The `.openSettings` observer is registered at app level (`ShunApp.init`), not
   on `SettingsView` content.
3. Activation policy is flipped to `.regular` on settings-open and on
   check-for-updates, and reverted to `.accessory` once no user-facing window
   remains.
4. No leaked `willClose` observers across repeated open/close cycles.

## 4. Problem Analysis

- **Approach A — keep `openWindow` + add only the `.regular` flip.** Fixes Cause A
  but leaves Cause B (dead subscriber after first close). Rejected: the report is
  intermittent precisely because of Cause B.
- **Approach B — full `ActivationPolicyCoordinator` (lease-counted).** Robust
  against overlapping windows but over-engineered for an app with two activation
  call sites. Rejected by KISS/YAGNI and the explicit project constraint.
- **Chosen approach — mirror Sokki, leak-free.** App-level, stateless
  `.openSettings` observer (in `ShunApp.init`) that flips policy and fronts the
  window via AppKit; one `AppDelegate`-owned `willClose` observer reverts to
  `.accessory` when the last user-facing window closes. The updates path reuses
  the same flip and the same revert. Fixes both causes with minimal machinery and
  no mutable state on the `App` value type.

## 5. Decision Log

**1. Where is the `.openSettings` observer registered?**
- Options: A) on `SettingsView` content (current) · B) in `ShunApp.init()` (app-level)
- Decision: **B)** — an app-level observer outlives the window, so a click after
  a close still reopens it. Directly fixes Cause B. Matches `SokkiApp.init`. The
  closure is stateless (captures only `NSApp` and the title literal), so it is
  safe regardless of `App` value-type semantics, and a (theoretical) double
  `init` would at worst front the window twice — idempotent.

**2. How is the window presented?**
- Options: A) `openWindow(id:"settings")` · B) `NSApp.windows` lookup by title + `makeKeyAndOrderFront` + `orderFrontRegardless`
- Decision: **B)** — `openWindow` is an `@Environment` value unavailable in a raw
  notification closure, and it is the mechanism tied to the dead subscriber. The
  `Window` scene persists its window in `NSApp.windows`; AppKit fronting is what
  Sokki uses and is reliable on Tahoe. Window title is `"Shun Settings"` (from
  `Window("Shun Settings", id: "settings")` — `ShunApp.swift:9`).

**3. How / when is the `.regular` flip reverted?**
- Options: A) never revert (Dock icon leaks) · B) timer · C) on each window's `willCloseNotification`, revert iff no user-facing window remains
- Decision: **C)** — *Revised in Phase 4 review (B2).* Originally the settings
  window reverted on its own `willClose` and the updates path used a separate
  one-shot. A one-shot keyed on the first `willClose` (`object: nil`) reverts on
  *any* window close — including the Settings window closing while a Sparkle
  window is still up, or an incidental menu/tooltip close before Sparkle's UI
  appears — yanking `.accessory` out from under a visible window (the exact bug).
  Instead, a single observer checks **end state**: on any `willClose`, revert to
  `.accessory` only if no `window.isVisible && window.canBecomeMain` window
  remains. Checking end-state (not the closing window's identity) is immune to
  incidental closes. `canBecomeMain` admits the Settings window and the Sparkle
  update window but excludes `NSStatusBarWindow` (not main-able) — so the app
  does not get stuck in `.regular`. Sparkle's brief "you're up to date" alert is
  modal (blocks the main thread), so the deferred check runs only after it is
  dismissed. Note: `canBecomeMain == true` for Sparkle's own update/progress
  window is *assumed, not verified from source* (Sparkle builds that UI
  internally; `UpdaterService.swift` only wraps `SPUStandardUpdaterController`).
  If Sparkle uses a non-main-able `NSPanel`, the predicate would revert too early
  on the updates path. The manual Tahoe test (Verification step 5) is the gate
  that confirms this empirically before shipping.

**4. How is the revert observer kept from leaking, and where does it live?**
- Options: A) re-add on every open (Sokki — accumulates duplicates) · B) guard `var` stored on `ShunApp` · C) one observer owned by `AppDelegate`, registered once
- Decision: **C)** — *Revised in Phase 4 review (B1).* `ShunApp` is a SwiftUI
  `struct` (value type, `ShunApp.swift:4`); a stored guard `var` on it cannot
  reliably hold register-once state, and a captured mutable local is illegal in
  the `@Sendable` NotificationCenter block under Swift 6. `AppDelegate` is a
  `@MainActor` reference type and the existing notification hub — register the
  single `willClose` observer there, exactly once in
  `applicationDidFinishLaunching`. Storing the returned token is optional (it
  lives for the whole app lifetime); registering once in `didFinishLaunching`
  (which runs once) inherently satisfies AC4.

**5. Activation handling for the Sparkle "Check for Updates" path.**
- Options: A) leave as-is (`NSApp.activate()` only) · B) add the same `.regular` flip and reuse the shared revert
- Decision: **B)** — *Revised in Phase 4 review (B2).* Augment the existing
  `observeUpdateChecks` handler (`AppDelegate.swift:145-149`): add
  `NSApp.setActivationPolicy(.regular)` before its existing `NSApp.activate()` +
  `updater?.checkForUpdates()`, keeping the existing `[weak self]` +
  `Task { @MainActor in }` shape (N2). No separate revert observer — the shared
  observer from decision 4 reverts once the Sparkle window closes. Residual edge:
  if the user cancels the check before any Sparkle window ever appears and no
  other user-facing window is open, `.regular` may linger until the next window
  open/close (or the menu closing, which also triggers the shared check). Rare
  and self-healing; accepted under YAGNI.

**6. Keep `@Environment(\.openWindow)` in `ShunApp`?**
- Options: A) keep · B) remove
- Decision: **B)** — unused once presentation moves to AppKit; removing it avoids
  a dead dependency and signals the new mechanism.

## 6. Design

### `ShunApp.swift`

Add an `init()` that registers a single app-level observer for `.openSettings`
with `queue: .main`. The handler, wrapped in `MainActor.assumeIsolated`:

1. `NSApp.setActivationPolicy(.regular)`
2. `NSApp.activate(ignoringOtherApps: true)`
3. `NSApp.windows.first { $0.title == "Shun Settings" }`, then
   `makeKeyAndOrderFront(nil)` + `orderFrontRegardless()`.

The closure captures no instance state. Remove `@Environment(\.openWindow)` and
the `.onReceive(.openSettings)` modifier from the scene content. `SettingsView`
keeps its environment objects and `.onDisappear` cache invalidation unchanged.

Concurrency (N1): `MainActor.assumeIsolated` is sound because the observer is
registered with `queue: .main`, so the block is **delivered** on the main thread
— this is the guarantee, independent of which thread posts the notification.
This mirrors `SokkiApp.init` (`SokkiApp.swift:18-21`).

### `AppDelegate.swift`

- `openSettings()`: drop the bare `NSApp.activate()`; keep only the
  `NotificationCenter.post(.openSettings)`. All three entry points
  (`StatusItemController` menu callback `AppDelegate.swift:27-29`, first-launch
  auto-open `37-42`, `applicationShouldHandleReopen` `65-68`) funnel through this
  method, so all are fixed at once; the app-level observer now performs the
  `activate(ignoringOtherApps:)`.
- `applicationDidFinishLaunching`: register **one** observer for
  `NSWindow.willCloseNotification` (`object: nil`, `queue: .main`). Its handler
  defers one main-actor turn (via `Task { @MainActor in }`, so the closing window
  has finished ordering out), then:
  `if !NSApp.windows.contains(where: { $0.isVisible && $0.canBecomeMain }) { NSApp.setActivationPolicy(.accessory) }`.
- `observeUpdateChecks()`: in the existing handler, add
  `NSApp.setActivationPolicy(.regular)` immediately before the existing
  `NSApp.activate()` + `updater?.checkForUpdates()`. Keep the `[weak self]` +
  `Task { @MainActor in }` shape (N2). The shared `willClose` observer reverts.

### Activation-policy safety

`.accessory` reverts are idempotent. There is exactly one revert site (the shared
`willClose` observer) and two flip sites (settings open, updates check); the flip
sites never need to coordinate because the revert is end-state-driven, not
event-counted. No coordinator is introduced.

## 7. Files Changed

- `Sources/Shun/ShunApp.swift` — add `init()` with an app-level, stateless
  `.openSettings` observer (`queue: .main`) that flips to `.regular`, activates,
  and fronts the `"Shun Settings"` window via AppKit; remove
  `@Environment(\.openWindow)` and the in-view `.onReceive(.openSettings)`.
- `Sources/Shun/AppDelegate.swift` — `openSettings()` posts only; register one
  `NSWindow.willCloseNotification` observer in `applicationDidFinishLaunching`
  that reverts to `.accessory` when no `isVisible && canBecomeMain` window
  remains; `observeUpdateChecks()` adds the `.regular` flip before activating.

## 8. Verification

1. [AC1] `swift build` completes without errors.
2. [AC2] Grep confirms the `.openSettings` observer is in `ShunApp.init` and no
   `.onReceive(.openSettings)` / `openWindow` remains in `ShunApp.swift`.
3. [AC3] Package the app (`./Scripts/package_app.sh`) and manually verify on
   macOS Tahoe: clicking "Settings…" brings the window frontmost and key, even
   after closing it once; after the window (and any Sparkle dialog) closes, the
   Dock icon disappears (policy back to `.accessory`).
4. [AC4] Code review confirms the `willClose` observer is registered exactly once
   (in `applicationDidFinishLaunching`), so repeated open/close adds no observers.
5. [B2 regression] With Settings open, trigger "Check for Updates"; closing the
   Settings window first must not drop the Dock icon while a Sparkle window is
   still visible (the shared predicate keeps `.regular` until the last
   `canBecomeMain` window closes).

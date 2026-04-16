# Fix: Hotkey Toggle Breaks After Imperfect Hide

## Problem

Repro with Neovate bound to cmd+3:

1. Press cmd+3 → Neovate activates. ✓
2. Press cmd+3 → Neovate's windows disappear, **but** the menu bar still shows "Neovate".
3. Press cmd+3 → nothing happens (expected: re-show Neovate).

## Root Cause

`TapContext.matchAndResolve` (`Sources/Shun/Services/ShortcutManager.swift:89-111`) uses "is frontmost" as a proxy for "is visible":

```
frontmost == target  → .hide
else running         → .activate
else                 → .launch
```

For some apps (commonly Electron-based, Neovate included), `NSRunningApplication.hide()` hides the windows via Apple Event but does **not** transfer frontmost to another app. macOS still considers the app frontmost; no `didActivateApplicationNotification` fires, so our cached `_frontmostApp` stays pointing at the now-hidden app. On the next press we match frontmost again, re-issue `.hide` on an already-hidden app, and the user sees nothing.

The Carbon fallback path (`ShortcutManager.swift:459-481`) reads `workspace.frontmostApplication` live rather than from our cache, but macOS itself reports the same stale frontmost — so the bug is identical there.

## Decision Log

**1. Signal for "should we hide or show?"**
- A) Keep using `frontmost == target` alone — status quo, broken
- B) Add `isHidden` check: if target is the frontmost-matching app AND already hidden, show it instead of hiding it again
- C) Abandon the cache, query `NSWorkspace.shared.frontmostApplication` on every event

Decision: **B**. `isHidden` is the actual visibility signal. C still doesn't fix the bug (macOS itself reports the stale frontmost) and adds workspace calls on the tap thread, which the current design deliberately avoids.

**2. Stale-cache recovery after a hide action**
- A) Leave cache as-is and rely solely on the `isHidden` check next press
- B) Optimistically clear `_frontmostApp` inside the same lock acquisition that returns `.hide`

Decision: **B**. Belt-and-suspenders: if for any reason macOS *does* move frontmost after hide, our stale cache entry is still incorrect to keep. Not applicable to the Carbon path (see #4).

**Why optimistic clearing is safe in both app behaviors:**
- Cooperative app (hide transfers frontmost): macOS fires `didActivateApplicationNotification` for the new frontmost; our observer overwrites the nil we just wrote. No visible effect.
- Uncooperative app (hide leaves frontmost stale): the cache stays nil, so the next press skips the frontmost-match branch and falls through to `_runningApps[id]` → `.activate`. Exactly the behavior we want.

The only "wrong" scenario would be a rapid double-press arriving before macOS processes the hide; in that case press-2 sees a nil cache and `.activate`s, netting out to "show" instead of "still hidden". Rapid double-press of a toggle is undefined anyway.

**3. `.activate` un-hides — no separate unhide call needed**
Both code paths to activation (`_fastActivate` via SkyLight `_SLPSSetFrontProcessWithOptions`, and the `NSRunningApplication.activate()` fallback) un-hide the target app as a side effect of bringing it to front. So returning `.activate` for a hidden-but-running app is sufficient; we do not need an explicit `unhide()`.

**4. Why Part B does not apply to the Carbon fallback**
The Carbon path reads `workspace.frontmostApplication` live on every press — there is no cache of our own to clear. Live reads are always in sync with what macOS believes, which for the uncooperative case is "still Neovate even though hidden". Part A (`isHidden` guard) is therefore both necessary and sufficient on this path.

**5. Thread safety of `isHidden` on the tap thread**
`NSRunningApplication.isHidden` is a KVO-backed property read, same class and pattern as the existing `isTerminated` check already inside the `OSAllocatedUnfairLock`. Safe.

## Fix

**Change 1 — `TapContext.matchAndResolve` (CGEvent tap path)**

Inside the `frontmost matches target` branch:

```swift
if let frontmost = _frontmostApp,
   frontmost.bundleIdentifier == entry.bundleIdentifier
{
    if frontmost.isHidden {
        log.info("stale frontmost: \(entry.bundleIdentifier) is hidden, activating")
        _frontmostApp = nil
        return .activate(frontmost)
    }
    _frontmostApp = nil
    return .hide(frontmost)
}
```

`log.info` (not `log.debug`) is used on purpose: `debug`-level messages are filtered out of the default unified log store and evaporate quickly. `info` persists long enough that a user who hits the bug can later run `log show --predicate 'subsystem == "com.shun"'` and still see the breadcrumb — which is the whole point of logging it.

**Change 2 — `registerCarbon` (Carbon fallback)**

Mirror the `isHidden` guard; no cache to clear:

```swift
if let frontmost = workspace.frontmostApplication,
   frontmost.bundleIdentifier == bundleIdentifier,
   !frontmost.isHidden
{
    frontmost.hide()
} else {
    // existing activate / launch branch, unchanged
}
```

## Non-Goals / Scope

- `.hide` still does not log usage; the log-on-activate / log-on-launch rule is unchanged.
- **Subtle behavior change to be aware of**: on the uncooperative-app repro, press-3 previously returned `.hide` (no usage log) and now returns `.activate` (logs a usage event). This is arguably more correct — the user really did cause the app to surface — but it means usage counts for Electron-ish apps may tick up slightly versus pre-fix history. Flagging here so the Insights tab's numbers don't look mysterious later.
- Cold-launch branch unchanged.
- No change to how we issue `hide()` — we accept that some apps don't transfer frontmost; this fix makes us resilient to that instead of fighting it.
- **Apps that no-op `hide()` entirely** (ignore the Apple Event): `.hide` becomes a no-op but we clear the cache; next press goes to `.activate`, which re-focuses an already-visible app. Harmless, not worth special-casing.
- **Mirror case — cached frontmost is wrong in the *other* direction** (cache says not-frontmost or nil, but app is actually frontmost-and-visible): we deliberately do not try to detect this and issue `.hide`. The request lands in the `_runningApps[id]` branch and returns `.activate`, which re-focuses an already-focused app (a no-op from the user's perspective). When our cache and macOS disagree, we err on the side of show-over-hide: hiding an app the user can see is disruptive; re-activating one they can already see is a no-op.

## Manual Test Plan

Split by behavior class to cover both the bug and the non-regression case:

1. **Uncooperative app (original repro)** — bind cmd+3 to Neovate or another Electron app.
   - cmd+3 → shows. cmd+3 → hides (menu bar may still say the app name — that's macOS). cmd+3 → shows again. ← previously broken.

2. **Cooperative app (regression guard)** — bind cmd+2 to Safari or TextEdit.
   - cmd+2 → shows. cmd+2 → hides, menu bar transfers to Finder/prev app. cmd+2 → shows again. No change from today's behavior.

3. **Hidden-via-cmd+H, then focus elsewhere** — bind cmd+4 to any app. Activate via cmd+4, press cmd+H, click Finder. Press cmd+4.
   - Expected: app re-shows. Exercises the `_runningApps` branch (frontmost is Finder, not our target), which already returns `.activate`. Verifies we haven't broken that path.

4. **Both event-tap and Carbon paths** — #1–3 should pass under the normal CGEvent tap. To exercise the Carbon fallback, temporarily deny Accessibility permission so `CGEvent.tapCreate` fails, then repeat #1 and #2. (This is an occasional check, not required on every change.)

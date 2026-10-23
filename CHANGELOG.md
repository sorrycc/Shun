# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.2.0
- A redesigned **Settings** window: tabs with icons in a unified title bar, settings grouped in cards with System Settings-style icons, and small animations as you switch tabs, record keys or add apps.
- Shortcuts show their keys as keycaps. While recording, the recorder shows the modifiers you hold, and a newly added app lights up in the list.
- Removing a shortcut no longer asks first: it shows **Undo** instead. Right-click a shortcut to show its app in Finder or remove it.
- **Insights** adds totals for the week and today, and a chart of the last 7 days.
- **General** shows whether Shun has the Accessibility permission, with a button to grant it.
- The app picker ranks names that start with what you type first, and shows keyboard hints.
- App icons load in the background, so lists scroll smoothly, and usage counts no longer scan the whole history on every redraw.
- ⌘1–⌘4 switch between the Settings tabs.
- **Shortcuts** sits in a card like the other tabs. The selected shortcut gets a soft highlight instead of a solid blue bar, rows highlight on hover and show a drag handle, and the toolbar groups its buttons and shows a status dot.
- Pages slide in from the direction of the tab you choose. The app picker's highlight glides between results, and the bars of the 7-day chart highlight on hover.
- **General** ends with Shun's mark and version. **Privacy** explains what stays on your Mac even when telemetry is off.
- Reopening Settings shows your apps' icons at once instead of loading them again.
- The menu bar menu lists your shortcuts with their keys: choose one to bring its app forward.
- Under a short list of shortcuts, a hint tells you that you can drag apps in from Finder.
- **Shortcuts** shows a banner while shortcuts are paused, with **Resume**, or while Shun lacks the Accessibility permission, with a button to grant it. While paused, the list dims and the toolbar says so.
- Recorded keys sit in a soft well instead of a bordered box. The window title changes with a short fade as you switch tabs, and the Accessibility status pops when it changes.
- **Insights** shows your daily average under today's count, and as a line across the 7-day chart.
- While you record keys, the row tells you how, and a key pressed without a modifier shakes the recorder instead of doing nothing. Right-click a shortcut to turn it off or clear its keys.
- The app picker shows placeholder rows while it scans your apps, has a button to clear the search, and shakes an app's **Added** badge if you pick it again.
- **Insights** compares today with your daily average, labels the chart's average line, rounds the bars at the top, and shows each app's share of your switches. The shortcut list shows how many apps it has, the switch of a shortcut whose app is missing fades, the active count rolls as it changes, and the tabs' tooltips show their ⌘1–⌘4 keys.
- With **Reduce Motion** on, the recorder and the app picker stop animating while they wait. Settings redraws with less work: usage charts are counted only after a change, and app paths are no longer parsed on every frame.
- The Settings window is a little taller, so Insights fits without scrolling as much. Settings no longer redraws while hidden on every shortcut you press, and checks the Accessibility permission at most once a second.

## 1.1.0
- Shun is rewritten in Go with a native UI drawn by MyGo: one small binary that uses less memory while it waits in the menu bar. Your shortcuts, their keys, the **Enabled** switch, usage history and privacy choices carry over, and you can still go back to an earlier version.
- Updates now come from Shun's GitHub releases, with a new update window, and download only what changed since your version.
- Granting the Accessibility permission takes effect at once, without relaunching Shun; until then shortcuts still work, more slowly.
- Fix: **Import Recipe** no longer counts shortcuts whose keys another shortcut has twice in its summary.


## 1.0.10 - 2026-06-29
- Fix: the **Settings** window now reliably appears on macOS 26 (Tahoe). Previously a second "Settings…" click could do nothing, and on Tahoe the window sometimes failed to come to the front.


## 1.0.9 - 2026-04-23
- Fix: **Insights** no longer over-counts hotkey activations. Holding a shortcut now registers as a single event instead of one per keyboard repeat, and existing inflated history is collapsed on next launch.


## 1.0.8 - 2026-04-22
- New **Privacy** tab in Settings. Opt-out anonymous crash-report upload and daily DAU ping (both default ON) to a self-hosted worker at `telemetry.sorrycc.dev`. No personal data or keystroke contents are sent.


## 1.0.7 - 2026-04-22
- Auto-update feed moved to the custom domain `download.sorrycc.dev`, served by a new Cloudflare Worker (repo: `sorrycc/downloads-worker`). Already-installed builds continue to receive updates via the original `r2.dev` host, which stays enabled as a fallback.
- Fix: "Check for Updates…" from the menu bar now brings Shun to the foreground so the Sparkle update window is key and frontmost (no extra click needed).


## 1.0.6 - 2026-04-22
- Hotkeys now pass through when Screen Sharing is frontmost, so keystrokes reach the remote Mac instead of triggering Shun's local toggle. Blocklist is hardcoded for now; may become configurable later.


## 1.0.5 - 2026-04-22
- Fix: Shortcut Recorder no longer crashes when configuring a shortcut. The `KeyboardShortcuts` resource bundle is now embedded in `Contents/Resources/` of the packaged `.app`, and SwiftPM's generated accessor is patched at build time to look there (GH #1).


## 1.0.4 - 2026-04-20
- Fix: app launches again on clean machines. 1.0.3 still crashed at startup with `could not load resource bundle` because the SwiftPM-generated menu bar icon bundle was never copied into the `.app`. Menu bar icons now ship as loose PNGs in `Contents/Resources/`. If your current version crashes on launch, auto-update can't reach you — please download this version manually.


## 1.0.3 - 2026-04-18
- Fix: app launches again. 1.0.2 crashed on startup because `Sparkle.framework` couldn't be located (missing `@executable_path/../Frameworks` rpath). If you're on 1.0.1, download 1.0.3 manually — 1.0.2 never worked.


## 1.0.2 - 2026-04-18
- Auto-update: Shun now checks for new versions once a day and offers a one-click install. Powered by [Sparkle](https://sparkle-project.org/).
- New "Check for Updates…" item in the menu bar and in Settings → General.
- Privacy: update checks don't send any system profile data (`SUEnableSystemProfiling=NO`).

## 1.0.1 - 2026-04-16

- New app icon with multiple design variants.
- Custom menubar icon with proper @2x/@3x scales.
- Developer ID signing + Apple notarization pipeline.
- Bun-based release orchestration (`bun release patch`).

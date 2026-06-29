# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased


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

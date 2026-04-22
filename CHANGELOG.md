# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased
- Fix: app no longer crashes the first time a shortcut Recorder is shown. `KeyboardShortcuts`'s SwiftPM-generated resource bundle wasn't being copied into the `.app`, so `Bundle.module` tripped a `fatalError` during SwiftUI layout. `Scripts/package_app.sh` now embeds every SPM dependency resource bundle from `.build/release/`. If your current version crashes, auto-update can't reach you — please download this version manually.


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

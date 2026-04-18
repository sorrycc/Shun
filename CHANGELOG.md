# Changelog

All notable changes to Shun are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

- Auto-update: Shun now checks for new versions once a day and offers a one-click install. Powered by [Sparkle](https://sparkle-project.org/).
- New "Check for Updates…" item in the menu bar and in Settings → General.
- Privacy: update checks don't send any system profile data (`SUEnableSystemProfiling=NO`).

## 1.0.1 - 2026-04-16

- New app icon with multiple design variants.
- Custom menubar icon with proper @2x/@3x scales.
- Developer ID signing + Apple notarization pipeline.
- Bun-based release orchestration (`bun release patch`).

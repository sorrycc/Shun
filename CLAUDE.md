# Shun — Claude Code Project Guide

## What is this?
macOS 14+ menu bar app for global hotkey-to-app switching (Thor clone). SPM-only, Swift 6 strict concurrency, hybrid SwiftUI + AppKit.

> Formerly named **HotApp** — renamed to **Shun** (瞬, "instant"). Legacy design docs under `docs/` still use the old name.

## Agent rules (read first)
- **Do not run `bun release` / `Scripts/release.ts` / `Scripts/release.sh`.** Releases are human-driven. `.claude/settings.json` denies these.
- **Do not edit `VERSION` or `.env`.** Denied by settings. `Scripts/sparkle.config` is editable but treat changes carefully — it's the source of truth for `SUFeedURL` baked into Info.plist; never rotate `SPARKLE_PUBLIC_KEY` without losing auto-update for installed users.
- **Do not rewrite `CHANGELOG.md`'s released sections.** You may add bullets to `## Unreleased`.
- **Do not rename legacy `HotApp` references in `docs/`.** They are historical.
- **Do not add `resources:` to the Shun SPM target in `Package.swift`.** See the SPM caveat below — the app will crash on other machines.
- **Do not change the `hotapp_` KeyboardShortcuts name prefix.** Requires a data migration.
- To verify a packaged build, invoke the `verify-release` agent instead of running codesign/stapler commands ad-hoc.

## Build & Run
```bash
swift build                                 # debug build (Sparkle skipped — no SUFeedURL)
swift build -c release                      # release build
./Scripts/package_app.sh                    # create .app bundle (release + Sparkle embed + codesign)
./Scripts/release.sh                        # notarize + DMG + zip + sha256 (uses ./VERSION)
bun release patch                           # full release: bump VERSION + CHANGELOG + release.sh + commit + tag
bun release patch --push --upload           # ALSO publish to R2 (appcast + DMG + zip + latest alias)
```

Version lives in `VERSION` (single line). CHANGELOG.md uses Keep-a-Changelog — edit `## Unreleased` as you work; `bun release` auto-promotes it to `## <version> - <UTC date>`.

Preferred release flow:
```bash
# 1. Edit CHANGELOG.md's "## Unreleased" section with release notes
# 2. Run:
bun release patch --push --upload   # bump, build, notarize, commit, tag, upload, push
```
Preflight refuses dirty working tree, duplicate tags, or an empty `## Unreleased` body (when --upload). On pipeline failure, reverts `VERSION` + `CHANGELOG.md`.

Lower-level: `./Scripts/release.sh` alone rebuilds/re-notarizes without bumping or tagging.
Override for throwaway test builds: `VERSION=1.0.0-test ./Scripts/release.sh`.

Design docs:
- `docs/designs/2026-04-16-signing-and-notarization.md` — Developer ID + notary pipeline
- `docs/designs/2026-04-17-r2-upload-and-auto-update.md` — Sparkle + R2 upload

### Release one-time setup
1. **Notary credentials** in keychain:
   ```bash
   xcrun notarytool store-credentials shun-notary \
       --apple-id <your-apple-id> \
       --team-id KU8S35TEW8 \
       --password <app-specific-password-from-appleid.apple.com>
   ```
2. **Sparkle tools** (for EdDSA signing):
   ```bash
   brew install --cask sparkle
   # NOTE: the cask bundles the tools under /opt/homebrew/Caskroom/sparkle/<ver>/bin/
   # but Homebrew does not symlink cask binaries to PATH. Run the full path:
   "$(ls -d /opt/homebrew/Caskroom/sparkle/*/bin | sort | tail -1)/generate_keys"
   ```
   Private key → login keychain; public key printed to stdout. Paste the public key into `Scripts/sparkle.config` as `SPARKLE_PUBLIC_KEY`. **Back up the private key** (export from keychain → password manager); losing it means you can never ship another update.

   `Scripts/upload_r2.ts` auto-locates `sign_update` at release time, so you don't need to put it on PATH.
3. **R2 bucket** (Cloudflare dashboard):
   - Create bucket `downloads` → Settings → enable `r2.dev` public access (kept on for legacy 1.0.4/1.0.5 installs whose `SUFeedURL` is baked to the r2.dev host).
   - Public host is `https://download.sorrycc.dev`, served by the `downloads-worker` repo at `~/Projects/downloads-worker` (Cloudflare Worker, R2 binding, custom-domain route). Deploy with `bun run deploy` in that repo.
   - `Scripts/sparkle.config` uses `SPARKLE_FEED_URL=https://download.sorrycc.dev/shun/appcast.xml` and `SPARKLE_PUBLIC_BASE_URL=https://download.sorrycc.dev`.
   - Create API token (Object Read & Write, scoped to `downloads`) → copy Access Key ID + Secret.
4. **R2 credentials** in `.env` at repo root (gitignored — Bun auto-loads `.env` from cwd, not `Scripts/`):
   ```
   R2_ACCOUNT_ID=...
   R2_ACCESS_KEY_ID=...
   R2_SECRET_ACCESS_KEY=...
   R2_BUCKET=downloads
   APP_KEY_PREFIX=shun
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
- `Sources/Shun/Services/UpdaterService.swift` — Sparkle `SPUStandardUpdaterController` wrapper
- `Sources/Shun/Services/RecipeService.swift` — hotkey recipe presets
- `Sources/Shun/Models/Recipe.swift`, `Models/UsageEvent.swift`, `Models/UsageStore.swift`
- `Sources/Shun/Views/SettingsView.swift` — root tabs container
- `Sources/Shun/Views/ShortcutListView.swift` — main shortcuts list + rows
- `Sources/Shun/Views/AppSearchView.swift` — app picker sheet
- `Sources/Shun/Views/InsightsView.swift` — usage insights tab

## Critical Conventions
- **Swift 6 strict concurrency**: all UI types are `@MainActor`. `AppShortcut` is `Sendable`.
- **LSUIElement app** (no Dock icon): must use `Window` scene, NOT `Settings` scene (it silently fails for LSUIElement apps)
- **Settings window bridge**: AppDelegate posts `.openSettings` notification → ShunApp listens with `.onReceive()` → calls `openWindow(id: "settings")`
- **KeyboardShortcuts dynamic names**: each AppShortcut has a `shortcutName` ("hotapp_{uuid}") that maps to `KeyboardShortcuts.Name`. Prefix kept as `hotapp_` for backward compatibility with existing user data — do not change it without a data migration. Key combos stored in UserDefaults by KeyboardShortcuts, NOT in our JSON.
- **Dual persistence**: shortcuts.json in ~/Library/Application Support/Shun/ + UserDefaults for key combos and globalEnabled. On first launch after rename, PersistenceService moves the legacy `HotApp/` dir to `Shun/`.
- **No SPM resources on the Shun target**: do not add `resources:` to the Shun target in `Package.swift`. SwiftPM's generated accessor expects `Shun_Shun.bundle` at `Bundle.main.bundleURL`, which for a hand-packaged `.app` is the `.app` root — a non-standard location that `Scripts/package_app.sh` does not produce, so the app ships broken. The dev machine's `.build/` path masks this (it's the accessor's fallback), so the crash only shows up on other machines. For new bundled assets, put files in the repo-root `Resources/` directory and copy them in `package_app.sh`; load them via `Bundle.main` / `NSImage(named:)`.
- **Orphan cleanup**: `cleanupOrphanedDefaults()` removes stale KeyboardShortcuts_ UserDefaults entries on load
- **Notification-driven refresh**: CRUD operations post `.shortcutsDidChange` → AppDelegate calls `shortcutManager.refreshRegistrations()`

## Dependencies
- `KeyboardShortcuts` (sindresorhus, v1.15.0) — global hotkey registration + SwiftUI Recorder view
- `Sparkle` (2.7.x, pinned to minor) — auto-update. `Sources/Shun/Services/UpdaterService.swift` wraps `SPUStandardUpdaterController`; instantiated only when `SUFeedURL` is in Info.plist (packaged builds, not `swift run`). Menu + Settings post `.checkForUpdates` notification → AppDelegate invokes the updater.

## Logs
Subsystem `com.shun` (Sparkle/UpdaterService uses `com.shun.Shun`). Known categories:
- `ShortcutManager` — registration + toggle logic
- `ShortcutRow` — per-row UI events
- `updater` — Sparkle (under subsystem `com.shun.Shun`)

```bash
log stream --predicate 'subsystem == "com.shun"' --level debug
log stream --predicate 'subsystem == "com.shun" AND category == "ShortcutManager"' --level debug
log show --predicate 'subsystem == "com.shun"' --last 1h --info --debug
```

## Design Doc
Full spec: `docs/designs/2026-03-18-hotapp.md` (uses the old name)

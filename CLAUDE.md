# Shun — Claude Code Project Guide

@AGENTS.md

## What is this?
macOS 14+ menu bar app for global hotkey-to-app switching (Thor clone). Go, with native UI drawn by
[MyGo](https://github.com/egoist/mygo) (package `ui`, no webview, no cgo); macOS APIs MyGo lacks are called through
purego. Same structure as `~/Projects/mayoi`.

> Formerly named **HotApp**, then written in Swift (≤ 1.0.10). Legacy design docs under `docs/` describe the Swift
> app and use the old name.

## Agent rules (read first)
- **Do not run `Scripts/bump.sh`, `Scripts/sparkle_bridge.sh`, `Scripts/upload_r2.ts`, `go tool mygo build -upload` or
  `go tool mygo keygen`.** Releases and keys are human-driven. `.claude/settings.json` denies these.
- **Do not edit `version` in `mygo.json`, or `.env`.** `Scripts/bump.sh` sets the version; `.env` is denied by settings.
- **Never rotate the keys:** `updates.publicKey` in `mygo.json` (MyGo updates) and `SPARKLE_PUBLIC_KEY` in `Scripts/sparkle.config`
  (the Swift installs' last update) — installed apps would stop updating.
- **Do not change the bundle ID `com.shun.Shun`, the app name `Shun`, or the Developer ID team `KU8S35TEW8`.** They key
  the data directory, the UserDefaults the migration reads, crash report names (`Shun-*.ips`), and the users'
  Accessibility permission.
- **Do not break the Swift formats** of `shortcuts.json` (required fields `id`, `bundleURL`, `bundleIdentifier`,
  `displayName`, `shortcutName`, `isEnabled`; uppercase UUIDs; directory file URLs with a trailing slash) and
  `usage.json` (ISO 8601 dates without fractional seconds): a user may go back to a Swift version.
- **Do not change the `hotapp_` prefix of `shortcutName`.** The migration looks up `KeyboardShortcuts_hotapp_<id>`.
- **Do not rewrite `CHANGELOG.md`'s released sections.** You may add bullets to `## Unreleased`.
- **Do not rename legacy `HotApp` references in `docs/`.** They are historical.
- To verify a packaged build, invoke the `verify-release` agent instead of running codesign/stapler commands ad-hoc.

## Build & Run
```bash
Scripts/dev.sh                                # "Shun Dev" with live reload, data seeded from the installed Shun; offers to quit it
go build ./... && go vet ./... && go test ./...
go tool mygo vet                              # MyGo's checks of build lifetimes in views
go tool mygo build -platform darwin/arm64     # build/darwin-arm64/Shun.app + .dmg, ad hoc signed
SHUN_DATA_DIR=/tmp/x go run ./cmd/shun         # any build against another data directory
SHUN_SHOTS=/tmp/shots go test ./cmd/shun        # PNGs of the Settings tabs, to look at
```
`Scripts/dev.sh` runs a bare binary (`.mygo/dev/shun`), rebuilt on change, not `go tool mygo dev`: macOS 26 keeps
the menu bar icon of mygo dev's "Shun Dev.app" bundle hidden. Its event tap has the terminal's Accessibility.

`mygo.json` is the configuration, as in mayoi: version, ad hoc signing, `LSUIElement`, and `updates` once the
public key is in. Release builds get the Developer ID and notarization patched into it by `.github/workflows/release.yml`.
Crash reports and settings sync go to Mon (`https://mon.sorrycc.dev`, or `SHUN_MON_URL`); dev builds send crash
reports only with `SHUN_MON_URL` set.

Releasing (human): `Scripts/bump.sh patch` → tag push → `.github/workflows/release.yml` builds, signs, notarizes and
publishes to GitHub releases, which installed apps update from. See CONTRIBUTING.md, including the one-time
Sparkle bridge for Swift installs.

## Architecture (key files)
- `cmd/shun/main.go` — app lifecycle, tray menu, Settings window (made lazily), activation policy, migration, telemetry start
- `cmd/shun/hotkeys.go` — wires the event tap (or the Carbon fallback via `mygo.GlobalShortcut`) to the engine and the store
- `cmd/shun/settings.go` — Settings root (tabs), shared layout helpers; `tabs.go` — General, Insights, Privacy, Account
- `cmd/shun/account.go` — settings sync with Mon: sign-in (device flow), token in the Keychain, push/pull/retry
- `cmd/shun/style.go` — the look: palette from the theme, title-bar tabs, icon tiles, keycaps, motion presets
- `cmd/shun/shortcuts.go` — Shortcuts tab: list (drag to reorder), rows, toolbar, drop, recipe import/export
- `cmd/shun/picker.go` — app picker modal (`mygo.Shell.InstalledApps`); `recorder.go` — shortcut recorder widget
- `cmd/shun/icons.go` — SVG icons, app icon cache (`mygo.Shell.FileIcon`, drawn in the background)
- `internal/hotkey` — `Engine`: shortcut table + running/frontmost app cache under one mutex; decides hide/activate/launch
- `internal/macos` — purego: event tap (`tap.go`), running apps, SkyLight fast activation, NSWorkspace observers,
  bundles (`apps.go`), CFPreferences reader for the migration (`defaults.go`), objc helpers (`objc.go`)
- `internal/store` — shortcuts (`store.go`, `shortcut.go`), prefs (`prefs.go`, settings.json), recipes (`recipe.go`),
  UserDefaults migration (`migrate.go`), files + HotApp dir move (`files.go`)
- `internal/usage` — 7-day activation log (usage.json), safe from any goroutine
- `internal/keys` — key combos: Carbon key code + Carbon modifiers, CGEvent flags, labels, accelerators
- `internal/mon` — copy of Mon's Go client (sign-in, settings documents, crash reports); keep in step with Mayoi's
- `internal/cloudsync` — sync engine: shortcuts by bundle ID + synced prefs, three-way merge, state in `sync.json`
- `internal/telemetry` — crash reports (.ips) to Mon, anonymous

## Critical Conventions
- **Threads**: MyGo runs the view, menus, tray and workspace notifications on the main thread. The event tap's
  callback runs on its own locked OS thread (`StartEventTap`) and must only touch `hotkey.Engine`, `macos` process
  calls and `usage.Store` (all thread-safe). `store.Store` is main-thread only; `store.Prefs` and `usage.Store` are
  mutex-guarded. From goroutines, change view state with `win.Update`, or `mygo.RunOnMain`.
- **purego**: pass Go pointers to C only from heap/package variables or through `//go:uintptrescapes` helpers
  (`send`); wrap objc calls off the main thread in `withPool`. Callbacks are created once (purego allows ~2000).
- **Accessibility**: an untrusted process gets an event tap that receives nothing (no error), so check
  `macos.IsTrusted()` before relying on the tap; `hotkeys.waitForPermission` switches from Carbon to the tap once
  granted. While Shun is the active app the engine is paused, so the recorder gets the keys.
- **LSUIElement app**: `ActivationPolicyAccessory`; opening a window flips to Regular (needed on Tahoe) and
  `refreshActivationPolicy` flips back once no window is visible.
- **Persistence**: everything in `~/Library/Application Support/Shun/` (`shortcuts.json` with combos, `settings.json`,
  `usage.json`, `updater.json` of MyGo's updater, `sync.json` of settings sync; dev builds keep Mon's token in
  `mon-token` there, release builds in the Keychain). The Swift app kept combos and prefs in UserDefaults;
  `store.Migrate` imports them once (flag `migratedFromUserDefaults`) and leaves them in place.
- **Store changes** call `OnChange` listeners → `hotkeys.refresh()` re-registers.
- **Views** are functions of state rebuilt every frame: keep `ui.Element` values out of app state; use `ui.Handle`,
  `ui.ListState`, `ui.Local` for persistent widget state. Test views with `ui.NewTester` (see `cmd/shun/view_test.go`).

## Dependencies
- `github.com/egoist/mygo` (replaced by `github.com/sorrycc/mygo`, as in mayoi) — app, windows, tray, dialogs,
  global shortcuts, installed apps, login item, native UI, updater (`plugins/updater/native`), build/sign/notarize/DMG
- `github.com/ebitengine/purego` — calls into AppKit, CoreGraphics, CoreFoundation, SkyLight without cgo

## Logs
Release builds: `~/Library/Logs/Shun/Shun.log` (`Shun.log.1` the run before). Dev builds: stderr.

## Menu bar icon missing
Not a code bug. macOS 26 can file Shun's icon under the terminal (e.g. iTerm2) that once launched it; if that
terminal is off in System Settings → Menu Bar → "Allow in the Menu Bar", Shun's icon stays hidden even with Shun's
own switch on. Fix: remove `com.shun.Shun` from the terminal's `menuItemLocations` in `trackedApplications` (a binary
plist) of `~/Library/Group Containers/group.com.apple.controlcenter/Library/Preferences/group.com.apple.controlcenter.plist`
(needs Full Disk Access), then `killall ControlCenter` and relaunch Shun. Open the release app from Finder or `open`,
not from a terminal.

## Design Doc
Original spec: `docs/designs/2026-03-18-hotapp.md` (Swift, old name). Behavior is preserved; see the Swift-era designs
for the reasoning behind the event tap, SkyLight activation, stale-frontmost handling and the Screen Sharing blocklist.

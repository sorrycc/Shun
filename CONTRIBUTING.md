# Contributing

## Development

```bash
Scripts/dev.sh              # run "Shun Dev" with live reload, on a copy of the installed Shun's data
Scripts/dev.sh --reseed     # copy the installed Shun's data again
go test ./...               # unit tests, and the Settings window's views, headless
go test -short ./...        # skip nothing slow yet, as CI runs it
SHUN_TAP_LATENCY=1 go test -run TapLatency -v ./internal/macos   # time the event tap (needs Accessibility)
SHUN_SHOTS=/some/dir go test ./cmd/shun                           # save PNGs of the Settings window's tabs
```

`Scripts/dev.sh` builds and runs a bare binary, `.mygo/dev/shun`, and rebuilds and restarts it as Go files, assets
or `mygo.config.ts` change, rather than `go tool mygo dev`: macOS 26 keeps the menu bar icon of mygo dev's
"Shun Dev.app" bundle hidden, even when System Settings allows it. Run from a terminal, it has the terminal's
Accessibility permission, so its event tap works; Launch at Login does not, as it needs a bundle. It keeps its data in `~/Library/Application Support/Shun Dev`, copied from the installed
Shun's the first time. The installed Shun takes the same keys, so when it runs
the script offers to quit it, and opens it again when Shun Dev stops. `SHUN_DATA_DIR=/some/dir` points any build at another data directory, e.g. a copy of
`~/Library/Application Support/Shun` to try a change on real shortcuts.

The event tap only receives the keys of other apps with the Accessibility permission, which macOS grants per
signature: an ad hoc signed build gets a tap that receives nothing, so Shun checks `AXIsProcessTrusted()` first and
falls back to global hotkeys (Carbon) until the permission is granted. Users of the Swift versions keep their
permission because release builds keep the bundle ID `com.shun.Shun` and the Developer ID of team `KU8S35TEW8`.

## Setting up a fresh clone

Releases need gitignored local files:

- `.env` — R2 credentials, only for the one Sparkle bridge release (below)
- `Scripts/telemetry.config` — telemetry endpoint + HMAC key for local release builds (CI takes them from secrets;
  without them, the app ships with no telemetry)

If you already have a working Shun clone on this machine, copy them over:

```bash
Scripts/sync_local_secrets.sh /path/to/existing/shun
```

Otherwise, fill them in by hand from `.env.example` and `Scripts/telemetry.config.example`.

## Releasing

Version lives in the single-line `VERSION` file. Write the release notes under `## Unreleased` in `CHANGELOG.md` and
commit them, then:

```bash
Scripts/bump.sh patch    # or minor, major, or a version such as 2.0.0
```

It sets `VERSION`, renames `## Unreleased` to `## <version> - <date>` (adding a new empty `## Unreleased`), commits
and tags `v<version>`, and after asking pushes `master` and the tag. The
[Release workflow](.github/workflows/release.yml) then builds arm64 and Intel, signed with the Developer ID and
notarized, uploads the disk images and their signed updates to the GitHub release, and publishes it. Installed apps
check for it once a day, and from the menu bar's **Check for Updates…**; the update window shows the changelog
section.

### One-time setup

1. **Update signing key.** Make a key pair in a directory of its own (`go tool mygo keygen` without `-o` writes to
   the directory other MyGo apps' keys live in):

   ```bash
   go tool mygo keygen -o ~/.config/shun-update-keys
   cp ~/.config/shun-update-keys/mygo-update.pub mygo-update.pub   # commit it
   ```

   Keep `mygo-update.key` in a password manager and in the `MYGO_UPDATER_PRIVATE_KEY` secret: installed apps accept
   only updates signed with it, so losing it strands them.

2. **Repository secrets** of `sorrycc/shun`:

   | Secret | What |
   |---|---|
   | `MYGO_UPDATER_PRIVATE_KEY` | the contents of `mygo-update.key` |
   | `MACOS_CERTIFICATE` | the Developer ID Application certificate and its key, as a base64 `.p12` |
   | `MACOS_CERTIFICATE_PASSWORD` | the password of the `.p12` |
   | `APPLE_ID`, `APPLE_TEAM_ID` | the Apple ID of the developer account, and `KU8S35TEW8` |
   | `APPLE_APP_PASSWORD` | an app-specific password of the Apple ID, from account.apple.com |
   | `TELEMETRY_ENDPOINT_URL`, `TELEMETRY_INGEST_KEY` | as in `Scripts/telemetry.config` |

3. **Public releases.** Installed apps read
   `https://github.com/sorrycc/shun/releases/latest/download/update-darwin-<arch>.json` without signing in, so the
   releases must be public: make the repository public, or publish them to a public repository of their own and set
   `updates.github` in `mygo.config.ts` to it. A private repository's releases are not found, and apps never update.

### Moving users of the Swift versions over (once)

Shun 1.0.10 and earlier update with Sparkle, from the appcast on R2. After the Release workflow published the first
Go version, publish that same version to the appcast too:

```bash
Scripts/sparkle_bridge.sh --dry-run   # then without --dry-run
```

It builds the version locally, signed with the Developer ID and notarized with the `shun-notary` profile
(`xcrun notarytool store-credentials shun-notary --apple-id <id> --team-id KU8S35TEW8 --password <app-password>`),
packs `build/Shun-<version>.zip` as Sparkle expects, and runs `Scripts/upload_r2.ts`, which signs it with the
Sparkle key (keychain slot `ed25519-shun`; `sign_update` from `brew install --cask sparkle`) and adds it to
`appcast.xml`. Sparkle checks that the new app is signed with the same Developer ID. Once installed, the Go app
updates from GitHub; the appcast and `Scripts/upload_r2.ts` are no longer needed after that.

What carries over: the shortcuts (`~/Library/Application Support/Shun/shortcuts.json`, read and written in the Swift
format), their keys (imported once from the `KeyboardShortcuts_hotapp_*` UserDefaults into `shortcuts.json`),
"Enabled", the telemetry choices and anonymous ID (into `settings.json`), the usage history, and the automatic
update checks choice. UserDefaults are left as they were, so going back to a Swift version works.

## Logs

Release builds log to `~/Library/Logs/Shun/Shun.log` (and the run before to `Shun.log.1`); `go tool mygo dev` and
`go run` log to the terminal.

```bash
tail -f ~/Library/Logs/Shun/Shun.log
```

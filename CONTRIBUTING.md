# Contributing

## Releasing

Version lives in the single-line `VERSION` file at repo root. The preferred release flow is `Scripts/release.ts`, which bumps the version, builds, notarizes with Apple, commits, tags, and (optionally) pushes.

### One-time setup

Create an app-specific password at https://appleid.apple.com → Sign-In and Security → App-Specific Passwords, then register it in your keychain:

```bash
xcrun notarytool store-credentials shun-notary \
    --apple-id <your-apple-id> \
    --team-id KU8S35TEW8 \
    --password <app-specific-password>
```

You also need the `Developer ID Application` certificate for team `KU8S35TEW8` in your login keychain.

### Cutting a release

```bash
bun release patch --push     # 1.0.0 -> 1.0.1, commit + tag + push
bun release minor            # local only (push manually when verified)
bun release major --push
bun release --help
```

(`bun release` is an alias for `./Scripts/release.ts` via `package.json`.)

The script refuses to run on a dirty working tree or when the target tag already exists. If the build or notarization fails mid-way, it reverts `VERSION` so the repo returns to its previous state.

A release takes ~3–5 min (most of that is Apple's notary service). On success, `build/` contains:

- `Shun-<VERSION>.zip` and `Shun-<VERSION>.dmg` — both notarized + stapled
- `Shun-<VERSION>.sha256` — integrity hashes for the zip and dmg
- `Shun-<VERSION>-notary.log` — Apple submission ID + status (keep for audit)

Upload the zip and dmg to GitHub Releases (or wherever you distribute from).

### Lower-level primitives

- `./Scripts/package_app.sh` — build + Developer ID sign only (no notarization). Fast; use for local installs.
- `./Scripts/release.sh` — full notarization pipeline without version bump or git. Use for re-notarizing the current version.
- `VERSION=1.0.0-test ./Scripts/release.sh` — throwaway test build with a one-off version.

Full design and rationale: `docs/designs/2026-04-16-signing-and-notarization.md`.

## Logs

The app logs via `os.Logger` under subsystem `com.shun`.

Live tail:
```bash
log stream --predicate 'subsystem == "com.shun"' --level debug
```

Past logs (last hour, including `.debug`/`.info`):
```bash
log show --predicate 'subsystem == "com.shun"' --last 1h --info --debug
```

Console.app: select your Mac in the sidebar, then filter `subsystem:com.shun`.

Narrow by category (e.g. `ShortcutManager`, `ShortcutRow`):
```bash
log stream --predicate 'subsystem == "com.shun" AND category == "ShortcutManager"' --level debug
```

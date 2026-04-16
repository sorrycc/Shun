# Developer ID Signing + Notarization for Shun

**Date**: 2026-04-16
**Status**: Design — not yet implemented
**Goal**: Ship Shun to other users without Gatekeeper warnings.

## Decisions

| Decision | Choice | Why |
|---|---|---|
| Distribution | Developer ID + notarization (not Mac App Store) | User has Developer ID Application cert installed; menu bar app with global hotkeys is awkward under MAS sandbox |
| Credentials | Keychain profile (`notarytool store-credentials`) | Zero secrets in repo, simplest local workflow |
| Artifacts | Both `.zip` and `.dmg`, both stapled | Zip for quick installs / auto-update tooling later; DMG for the "download from website" user flow |
| Notarization submissions | **One** — submit only the DMG, staple both artifacts | Tickets are cdhash-keyed, `stapler staple` retrieves each by cdhash. Halves release wall-clock |
| Entitlements | **None** — no `.entitlements` file | Shun needs zero hardened-runtime exceptions. Empty plist would be YAGNI |
| Version | Env-var override in `package_app.sh`, read back from Info.plist in `release.sh` via `plutil` | Single source of truth, no per-release script edits |
| Preflight checks | Verify cert + keychain profile exist before any slow work | Catches first-run config errors in <1s instead of after 30s of building |
| Notary timeout | `--timeout 20m` on `notarytool submit --wait` | Default is 3h — pathological Apple hiccups shouldn't hang the script all afternoon |
| DMG layout | Includes `/Applications` symlink for drag-install | Standard macOS convention; trivial to add |
| Submission audit trail | `tee` notarytool output to `build/Shun-${VERSION}-notary.log` | Keeps submission ID near the artifact for later `notarytool log <id>` lookups |
| Stapling fallback | If `stapler staple Shun.app` fails after DMG-only submission, resubmit app-zip and retry | Single-submission-DMG works in practice but isn't explicitly guaranteed by Apple docs |
| Integrity hashes | SHA-256 for `.zip` and `.dmg` written to `build/Shun-${VERSION}.sha256` | Standard release artifact for download verification |

## 1. Signing identity & hardened runtime

- **Cert**: `Developer ID Application: chen cheng (KU8S35TEW8)` (already installed in login keychain).
- **Hardened runtime**: `codesign --options runtime`. Required for notarization.
- **Secure timestamp**: `codesign --timestamp`. Required for notarization.
- **No entitlements file**: Shun only uses Carbon `RegisterEventHotKey` (via KeyboardShortcuts), `NSWorkspace`, and filesystem I/O in `~/Library/Application Support`. None require entitlements. Default hardened-runtime restrictions are fine.

If entitlements are ever needed later (e.g., `com.apple.security.automation.apple-events` for AppleScript), add a `Resources/Shun.entitlements` file and a `--entitlements` flag to the codesign call at that time — don't plumb it in speculatively.

## 2. Script structure

Two scripts — clear separation of fast vs slow:

- **`Scripts/package_app.sh`** (rewrite existing) — build + bundle + Developer ID sign + hardened runtime. ~10s. Run for local installs. No Apple round-trip.
- **`Scripts/release.sh`** (new) — calls `package_app.sh`, then DMG + **one** notarization submission + staple both + final zip. ~4 min (single Apple round-trip). Run when cutting a release.

Each script starts with:

```bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."   # always run from project root
```

Config (cert name, team ID, keychain profile name) lives as shell vars at the top of each script — single place to edit.

## 3. `Scripts/package_app.sh` (build + sign)

```bash
VERSION="${VERSION:-1.0.0}"   # env-var override: VERSION=1.0.1 ./Scripts/package_app.sh

swift build -c release
# assemble Shun.app (same layout as today — Info.plist uses $VERSION)
codesign --force --options runtime --timestamp \
         --sign "Developer ID Application: chen cheng (KU8S35TEW8)" \
         Shun.app
codesign --verify --strict Shun.app                # sanity check
```

No embedded frameworks (SPM statically links Swift packages into the single executable), so no inner-to-outer signing. Single codesign call.

Note: `spctl --assess` is NOT run here — it would fail until the app is notarized, and ad-hoc-signed local builds are still useful.

## 4. `Scripts/release.sh` (notarize + distribute)

```bash
# ---- preflight: fail fast on config problems BEFORE slow work ----
preflight() {
    security find-identity -v -p codesigning | grep -q "KU8S35TEW8" \
        || { echo "FAIL: Developer ID cert for team KU8S35TEW8 not in keychain"; exit 1; }
    xcrun notarytool history --keychain-profile shun-notary >/dev/null 2>&1 \
        || { echo "FAIL: 'shun-notary' keychain profile missing. Run: xcrun notarytool store-credentials shun-notary ..."; exit 1; }
}
preflight

./Scripts/package_app.sh   # produces signed (not yet notarized) Shun.app

# single source of truth for version — read from the bundle we just built
VERSION=$(plutil -extract CFBundleShortVersionString raw -o - Shun.app/Contents/Info.plist)

rm -rf build && mkdir -p build

# ---- build signed DMG with drag-to-/Applications layout ----
mkdir -p build/dmg-staging
cp -R Shun.app build/dmg-staging/
ln -s /Applications build/dmg-staging/Applications
hdiutil create -volname Shun -srcfolder build/dmg-staging -ov -format UDZO \
               "build/Shun-${VERSION}.dmg"
rm -rf build/dmg-staging
codesign --force --timestamp \
         --sign "Developer ID Application: chen cheng (KU8S35TEW8)" \
         "build/Shun-${VERSION}.dmg"

# ---- ONE notarization submission, bounded, logged ----
# Submitting the DMG covers both the DMG itself and the app inside.
# Tickets are keyed by cdhash; stapler fetches each artifact's ticket by cdhash.
# --timeout 20m caps wall-clock if Apple's notary service hiccups (default is 3h).
# tee captures the submission ID for later `notarytool log <id>` lookups.
xcrun notarytool submit "build/Shun-${VERSION}.dmg" \
    --keychain-profile shun-notary --wait --timeout 20m \
    | tee "build/Shun-${VERSION}-notary.log"

# ---- staple both artifacts ----
# Single-submission DMG covers the app inside via cdhash, BUT this isn't
# explicitly guaranteed by Apple docs — fall back to a separate app submission
# if stapling the app fails with "no ticket available".
if ! xcrun stapler staple Shun.app; then
    echo "WARN: stapling from DMG submission failed — submitting app separately"
    ditto -c -k --keepParent Shun.app build/app-for-notary.zip
    xcrun notarytool submit build/app-for-notary.zip \
        --keychain-profile shun-notary --wait --timeout 20m \
        | tee -a "build/Shun-${VERSION}-notary.log"
    xcrun stapler staple Shun.app   # retry
fi
xcrun stapler staple "build/Shun-${VERSION}.dmg"

# ---- zip the stapled app (distribution zip) ----
ditto -c -k --keepParent Shun.app "build/Shun-${VERSION}.zip"

# ---- SHA-256 checksums for distributed artifacts ----
(cd build && shasum -a 256 "Shun-${VERSION}.zip" "Shun-${VERSION}.dmg" \
    > "Shun-${VERSION}.sha256")

# ---- verify ----
verify Shun.app app
verify "build/Shun-${VERSION}.dmg" dmg
```

**Output** in `build/`:
- `Shun-<VERSION>.zip` and `Shun-<VERSION>.dmg` — both notarized + stapled
- `Shun-<VERSION>.sha256` — SHA-256 hashes for download verification
- `Shun-<VERSION>-notary.log` — notarization submission audit trail

## 5. `verify()` function (end of release.sh)

Fails loud on any regression:

```bash
verify() {
    local bundle="$1"
    local kind="$2"    # "app" or "dmg"

    # Hardened runtime flag present
    codesign -d -v "$bundle" 2>&1 | grep -q "flags=.*runtime" \
        || { echo "FAIL: hardened runtime missing on $bundle"; exit 1; }

    # Team ID correct
    codesign -d -v "$bundle" 2>&1 | grep -q "TeamIdentifier=KU8S35TEW8" \
        || { echo "FAIL: wrong team id on $bundle"; exit 1; }

    # Stapled ticket present
    xcrun stapler validate "$bundle" \
        || { echo "FAIL: no stapled ticket on $bundle"; exit 1; }

    # Gatekeeper accepts it
    if [[ "$kind" == "app" ]]; then
        spctl --assess --type execute -v "$bundle" \
            || { echo "FAIL: spctl rejected $bundle"; exit 1; }
    else
        spctl --assess --type open --context context:primary-signature -v "$bundle" \
            || { echo "FAIL: spctl rejected $bundle"; exit 1; }
    fi

    echo "OK: $bundle"
}

verify Shun.app app
verify "build/Shun-${VERSION}.dmg" dmg
```

## 6. One-time setup (run once, manually)

```bash
# 1. Create app-specific password at https://appleid.apple.com
#    → Sign-In and Security → App-Specific Passwords
# 2. Register it in the keychain:
xcrun notarytool store-credentials shun-notary \
    --apple-id "sorrycc@gmail.com" \
    --team-id "KU8S35TEW8" \
    --password "<app-specific-password>"
```

Scripts reference `shun-notary` by name. No secrets in the repo.

## 7. Failure modes

| Scenario | Behavior |
|---|---|
| Missing keychain profile | Caught by `preflight()` in <1s with actionable error message |
| Missing / revoked cert | Caught by `preflight()` in <1s; if somehow missed, `codesign` fails early in `package_app.sh` |
| Notarization rejection | `notarytool submit --wait` exits non-zero. Submission ID is in `build/Shun-${VERSION}-notary.log`; debug with `xcrun notarytool log <submission-id> --keychain-profile shun-notary` |
| Apple notary service slow / hung | `--timeout 20m` bounds the wait; script fails with non-zero exit rather than hanging indefinitely |
| Partial failure mid-release | Re-run is safe — `rm -rf build` at start wipes stale artifacts; `Shun.app` is rebuilt |
| Timestamp server unreachable | `codesign --timestamp` fails. Retry when network returns |
| Script run from wrong dir | Explicit `cd "$(dirname "${BASH_SOURCE[0]}")/.."` makes scripts location-independent |

## 8. Cutting a release — user workflow

```bash
# one-time:
xcrun notarytool store-credentials shun-notary ...

# every release:
VERSION=1.0.1 ./Scripts/release.sh
# → build/Shun-1.0.1.zip and build/Shun-1.0.1.dmg, notarized + stapled
```

## 9. Changes to the repo

| File | Change |
|---|---|
| `Scripts/package_app.sh` | **rewrite** signing block (Developer ID + hardened runtime, replaces ad-hoc); support `VERSION` env var; `cd` to project root |
| `Scripts/release.sh` | **new** — preflight + DMG (with `/Applications` symlink) + single bounded notarization + staple + zip + `verify()`; logs submission ID to `build/*-notary.log` |
| `.gitignore` | add `build/` |
| `CLAUDE.md` | add release workflow note (one-time credentials setup + `VERSION=x.y.z ./Scripts/release.sh`) |

## 10. Out of scope

- Mac App Store submission (different cert, needs sandbox entitlements).
- Sparkle / auto-update integration (would reuse the zip artifact; separate design).
- CI/CD (this design targets local-machine builds; App Store Connect API key would be the upgrade path).
- Automated version bumping (env var is enough for now).

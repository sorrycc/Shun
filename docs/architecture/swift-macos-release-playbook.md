# Swift macOS App Release Playbook

**Purpose**: reusable recipe for shipping a Swift Package Manager macOS app with Developer ID signing + Apple notarization + stapled DMG/zip artifacts.

**Status**: battle-tested on Shun. Copy to a new project, swap the placeholders, and it should work on day one.

**What you get**: `.zip` and `.dmg` artifacts that open on any modern Mac without Gatekeeper warnings, both offline-verifiable (stapled), both with SHA-256 hashes for download integrity. Full flow runnable as a single command: `bun release patch --push`.

---

## 1. Prerequisites

- Apple Developer Program membership ($99/yr) — required for a Developer ID certificate.
- **Developer ID Application** certificate installed in your login keychain. Get it from Xcode → Settings → Accounts → Manage Certificates → `+` → Developer ID Application. Or download from developer.apple.com → Certificates → `+` → Developer ID Application.
- macOS 14+ development machine with Xcode command line tools (`xcrun`, `codesign`, `hdiutil`, `notarytool`, `stapler`, `spctl`, `plutil` all ship with the CLT).
- Swift 6 + SPM project that builds a macOS executable (`swift build -c release` produces a binary in `.build/release/`).
- App-specific password for your Apple ID (see setup below).
- **Optional**: Bun, for the TypeScript orchestrator. The bash scripts alone are sufficient; Bun just adds the version-bump + git-tag wrapper.

---

## 2. One-time machine setup

### 2a. Register notarization credentials in keychain

Create an app-specific password at https://appleid.apple.com → Sign-In and Security → App-Specific Passwords. Then:

```bash
xcrun notarytool store-credentials <PROFILE_NAME> \
    --apple-id <your-apple-id> \
    --team-id <YOUR_TEAM_ID> \
    --password <app-specific-password>
```

Pick a memorable profile name (e.g. `shun-notary`, `myapp-notary`). This stores the credentials in the macOS keychain under that name. The release script references the profile by name — no secrets in your repo.

### 2b. Sanity check

```bash
# Cert should appear:
security find-identity -v -p codesigning | grep "Developer ID Application"

# Profile should list past submissions (empty list on first use):
xcrun notarytool history --keychain-profile <PROFILE_NAME>
```

---

## 3. What goes into the repo

| File | Purpose |
|---|---|
| `VERSION` | Single-line semver. Source of truth for build + filenames. |
| `Scripts/package_app.sh` | Build + bundle + Developer ID sign + hardened runtime. Fast (~10s). |
| `Scripts/release.sh` | Full pipeline: preflight → build → DMG → notarize → staple → zip → SHA-256 → verify. Slow (~3–5 min, Apple round-trip). |
| `Scripts/release.ts` | Bun orchestrator: bump VERSION + call release.sh + git commit + tag + optional push. |
| `package.json` | `bun release` alias. |
| `.gitignore` | `/build`, `/<APP_NAME>.app` |
| `Resources/AppIcon.icns` | Your app icon (optional but recommended). |

---

## 4. File templates

Placeholders to replace when copying to a new project:

| Placeholder | Example | Where used |
|---|---|---|
| `<APP_NAME>` | `Shun` | Script vars, bundle name |
| `<BUNDLE_ID>` | `com.shun.Shun` | `CFBundleIdentifier` |
| `<SIGN_IDENTITY>` | `Developer ID Application: chen cheng (KU8S35TEW8)` | Full common name of your cert |
| `<TEAM_ID>` | `KU8S35TEW8` | Your Apple team ID |
| `<NOTARY_PROFILE>` | `shun-notary` | Keychain profile name you registered |
| `<MIN_OS_VERSION>` | `14.0` | `LSMinimumSystemVersion` |
| `<LSUIELEMENT>` | `true` (menu bar) / omit (regular app) | Only for LSUIElement apps |

### 4a. `VERSION`

```
1.0.0
```

One line. No "v" prefix. Bumped by `Scripts/release.ts`.

### 4b. `Scripts/package_app.sh`

```bash
#!/bin/bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="<APP_NAME>"
BUNDLE_ID="<BUNDLE_ID>"
VERSION="${VERSION:-$(cat VERSION)}"
SHORT_VERSION="${VERSION}"
BUILD_DIR=".build/release"
APP_BUNDLE="${APP_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"
SIGN_IDENTITY="<SIGN_IDENTITY>"

echo "Building ${APP_NAME} ${VERSION} (release)..."
swift build -c release

echo "Creating app bundle..."
rm -rf "${APP_BUNDLE}"
mkdir -p "${CONTENTS}/MacOS"
mkdir -p "${CONTENTS}/Resources"

cp "${BUILD_DIR}/${APP_NAME}" "${CONTENTS}/MacOS/${APP_NAME}"
cp "Resources/AppIcon.icns" "${CONTENTS}/Resources/AppIcon.icns"

cat > "${CONTENTS}/Info.plist" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleDisplayName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleIdentifier</key>
    <string>${BUNDLE_ID}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${SHORT_VERSION}</string>
    <key>CFBundleExecutable</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string><MIN_OS_VERSION></string>
    <key>LSUIElement</key>
    <true/>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

echo "Code signing with Developer ID + hardened runtime..."
codesign --force --options runtime --timestamp \
         --sign "${SIGN_IDENTITY}" \
         "${APP_BUNDLE}"

echo "Verifying signature..."
codesign --verify --strict "${APP_BUNDLE}"

echo "Done! ${APP_BUNDLE} is signed (not yet notarized)."
```

Notes:
- `--options runtime` enables hardened runtime (required for notarization).
- `--timestamp` embeds a secure Apple timestamp (also required).
- No `--entitlements` because typical apps don't need any hardened-runtime exceptions. If you use JIT, DYLD env vars, AppleScript, etc., add a `Resources/<App>.entitlements` file and pass `--entitlements` to `codesign`.
- Omit `<key>LSUIElement</key><true/>` for a regular app with a Dock icon.
- SPM statically links Swift packages, so there are no embedded frameworks to sign separately. If you introduce an `.xcframework` or dylib later, sign inner-to-outer.

### 4c. `Scripts/release.sh`

```bash
#!/bin/bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="<APP_NAME>"
APP_BUNDLE="${APP_NAME}.app"
SIGN_IDENTITY="<SIGN_IDENTITY>"
TEAM_ID="<TEAM_ID>"
NOTARY_PROFILE="<NOTARY_PROFILE>"

# ---- preflight: fail fast on config problems BEFORE slow work ----
preflight() {
    echo "Preflight checks..."
    if ! security find-identity -v -p codesigning | grep -q "${TEAM_ID}"; then
        echo "FAIL: Developer ID cert for team ${TEAM_ID} not in keychain." >&2
        exit 1
    fi
    if ! xcrun notarytool history --keychain-profile "${NOTARY_PROFILE}" >/dev/null 2>&1; then
        echo "FAIL: '${NOTARY_PROFILE}' keychain profile missing." >&2
        echo "      Register once with: xcrun notarytool store-credentials ${NOTARY_PROFILE} ..." >&2
        exit 1
    fi
    echo "  ok: Developer ID cert found"
    echo "  ok: '${NOTARY_PROFILE}' notary profile found"
}
preflight

# ---- build + sign the app ----
./Scripts/package_app.sh

# Single source of truth for version — read from the bundle we just built
VERSION=$(plutil -extract CFBundleShortVersionString raw -o - \
          "${APP_BUNDLE}/Contents/Info.plist")
echo "Releasing ${APP_NAME} ${VERSION}..."

rm -rf build && mkdir -p build

DMG_PATH="build/${APP_NAME}-${VERSION}.dmg"
ZIP_PATH="build/${APP_NAME}-${VERSION}.zip"
NOTARY_LOG="build/${APP_NAME}-${VERSION}-notary.log"
SHA_PATH="build/${APP_NAME}-${VERSION}.sha256"

# ---- build signed DMG with drag-to-/Applications layout ----
echo "Building DMG..."
mkdir -p build/dmg-staging
cp -R "${APP_BUNDLE}" build/dmg-staging/
ln -s /Applications build/dmg-staging/Applications
hdiutil create -volname "${APP_NAME}" -srcfolder build/dmg-staging \
               -ov -format UDZO "${DMG_PATH}" >/dev/null
rm -rf build/dmg-staging

echo "Signing DMG..."
codesign --force --timestamp --sign "${SIGN_IDENTITY}" "${DMG_PATH}"

# ---- ONE notarization submission (covers DMG + app inside by cdhash) ----
echo "Submitting to Apple notary service (up to 20 min)..."
xcrun notarytool submit "${DMG_PATH}" \
    --keychain-profile "${NOTARY_PROFILE}" --wait --timeout 20m \
    | tee "${NOTARY_LOG}"

# ---- staple both artifacts ----
# DMG submission usually publishes tickets for the app inside too,
# but Apple doesn't explicitly guarantee it. Fall back to a separate
# app submission if stapling the app fails.
echo "Stapling ticket to app..."
if ! xcrun stapler staple "${APP_BUNDLE}"; then
    echo "WARN: stapling from DMG submission failed — submitting app separately"
    ditto -c -k --keepParent "${APP_BUNDLE}" build/app-for-notary.zip
    xcrun notarytool submit build/app-for-notary.zip \
        --keychain-profile "${NOTARY_PROFILE}" --wait --timeout 20m \
        | tee -a "${NOTARY_LOG}"
    rm -f build/app-for-notary.zip
    xcrun stapler staple "${APP_BUNDLE}"
fi

echo "Stapling ticket to DMG..."
xcrun stapler staple "${DMG_PATH}"

# ---- zip the stapled app (distribution zip) ----
echo "Creating distribution zip..."
ditto -c -k --keepParent "${APP_BUNDLE}" "${ZIP_PATH}"

# ---- SHA-256 checksums for distributed artifacts ----
echo "Computing SHA-256 checksums..."
(cd build && shasum -a 256 \
    "${APP_NAME}-${VERSION}.zip" \
    "${APP_NAME}-${VERSION}.dmg" \
    > "${APP_NAME}-${VERSION}.sha256")

# ---- verify ----
verify() {
    local bundle="$1"
    local kind="$2"    # "app" or "dmg"

    # Capture codesign output to avoid `set -o pipefail` + `grep -q` SIGPIPE issue
    local codesign_out
    codesign_out=$(codesign -d -v "$bundle" 2>&1)

    # Hardened runtime is executable-only; DMGs always report flags=0x0(none)
    if [[ "$kind" == "app" ]]; then
        if ! grep -q "flags=.*runtime" <<< "$codesign_out"; then
            echo "FAIL: hardened runtime missing on $bundle" >&2; exit 1
        fi
    fi
    if ! grep -q "TeamIdentifier=${TEAM_ID}" <<< "$codesign_out"; then
        echo "FAIL: wrong team id on $bundle" >&2; exit 1
    fi
    if ! xcrun stapler validate "$bundle" >/dev/null 2>&1; then
        echo "FAIL: no stapled ticket on $bundle" >&2; exit 1
    fi
    if [[ "$kind" == "app" ]]; then
        if ! spctl --assess --type execute -v "$bundle" >/dev/null 2>&1; then
            echo "FAIL: spctl rejected $bundle" >&2; exit 1
        fi
    else
        if ! spctl --assess --type open --context context:primary-signature -v "$bundle" >/dev/null 2>&1; then
            echo "FAIL: spctl rejected $bundle" >&2; exit 1
        fi
    fi
    echo "  ok: $bundle"
}

echo "Verifying artifacts..."
verify "${APP_BUNDLE}" app
verify "${DMG_PATH}" dmg

echo ""
echo "Release ${VERSION} ready:"
echo "  ${ZIP_PATH}"
echo "  ${DMG_PATH}"
echo "  ${SHA_PATH}"
echo "  ${NOTARY_LOG}"
```

### 4d. `Scripts/release.ts` (Bun orchestrator)

```typescript
#!/usr/bin/env bun

import { existsSync, readFileSync, writeFileSync } from "fs";
import { resolve } from "path";

type BumpType = "patch" | "minor" | "major";

interface ParsedArgs {
  help: boolean;
  bump?: BumpType;
  push: boolean;
}

function parseArgs(): ParsedArgs {
  const args = Bun.argv.slice(2);
  const out: ParsedArgs = { help: false, push: false };
  for (const a of args) {
    if (a === "-h" || a === "--help") out.help = true;
    else if (a === "--push") out.push = true;
    else if (a === "patch" || a === "minor" || a === "major") out.bump = a;
    else {
      console.error(`Unknown argument: ${a}`);
      process.exit(2);
    }
  }
  return out;
}

function showHelp(): void {
  console.log(`
Usage: ./Scripts/release.ts <patch|minor|major> [--push]

Orchestrates a full release:
  1. Verifies working tree is clean
  2. Bumps the VERSION file (semver)
  3. Runs Scripts/release.sh (build + notarize + DMG + zip + sha256)
  4. Creates git commit and tag v<version>
  5. Optionally pushes commit and tag to origin

Examples:
  ./Scripts/release.ts patch          # 1.0.0 -> 1.0.1, commit + tag (no push)
  ./Scripts/release.ts minor --push   # 1.0.1 -> 1.1.0, commit + tag + push
  ./Scripts/release.ts major          # 1.1.0 -> 2.0.0

Options:
  patch|minor|major   Semver component to bump (required)
  --push              Push commit and tag to origin after success
  -h, --help          Show this help
`);
}

function bumpVersion(current: string, type: BumpType): string {
  const parts = current.trim().split(".");
  if (parts.length !== 3 || !parts.every((p) => /^\d+$/.test(p))) {
    throw new Error(`VERSION file is not plain semver "X.Y.Z": "${current}"`);
  }
  let [major, minor, patch] = parts.map(Number);
  if (type === "major") { major++; minor = 0; patch = 0; }
  else if (type === "minor") { minor++; patch = 0; }
  else { patch++; }
  return `${major}.${minor}.${patch}`;
}

async function run(cmd: string[], capture = false): Promise<string> {
  const proc = Bun.spawn(cmd, {
    stdout: capture ? "pipe" : "inherit",
    stderr: capture ? "pipe" : "inherit",
    stdin: "inherit",
  });
  const exitCode = await proc.exited;
  if (exitCode !== 0) {
    throw new Error(`Command failed (exit ${exitCode}): ${cmd.join(" ")}`);
  }
  return capture ? await new Response(proc.stdout).text() : "";
}

async function tryRun(cmd: string[]): Promise<boolean> {
  const proc = Bun.spawn(cmd, { stdout: "pipe", stderr: "pipe" });
  return (await proc.exited) === 0;
}

async function main(): Promise<void> {
  const args = parseArgs();
  if (args.help) { showHelp(); process.exit(0); }
  if (!args.bump) {
    console.error("Error: must specify patch, minor, or major\n");
    showHelp();
    process.exit(2);
  }

  const projectRoot = resolve(import.meta.dir, "..");
  process.chdir(projectRoot);

  const versionFile = resolve(projectRoot, "VERSION");
  if (!existsSync(versionFile)) {
    throw new Error(`VERSION file not found at ${versionFile}`);
  }

  const dirty = (await run(["git", "status", "--porcelain"], true)).trim();
  if (dirty !== "") {
    console.error("Error: working tree is dirty. Commit or stash first.\n");
    console.error(dirty);
    process.exit(1);
  }

  const currentVersion = readFileSync(versionFile, "utf8").trim();
  const newVersion = bumpVersion(currentVersion, args.bump);
  const tag = `v${newVersion}`;

  if (await tryRun(["git", "rev-parse", "--verify", `refs/tags/${tag}`])) {
    console.error(`Error: tag ${tag} already exists.`);
    process.exit(1);
  }

  const branch = (await run(["git", "symbolic-ref", "--short", "HEAD"], true)).trim();

  console.log(`Releasing ${currentVersion} -> ${newVersion}`);
  console.log(`  branch: ${branch}`);
  console.log(`  tag:    ${tag}`);
  console.log(`  push:   ${args.push ? "yes" : "no (local only)"}\n`);

  writeFileSync(versionFile, `${newVersion}\n`);

  try {
    await run(["./Scripts/release.sh"]);
  } catch (e) {
    writeFileSync(versionFile, `${currentVersion}\n`);
    console.error(`\nRelease pipeline failed. Reverted VERSION to ${currentVersion}.`);
    throw e;
  }

  await run(["git", "add", "VERSION"]);
  await run(["git", "commit", "-m", `release: ${newVersion}`]);
  await run(["git", "tag", tag]);
  console.log(`\nCreated commit and tag ${tag}.`);

  if (args.push) {
    await run(["git", "push", "origin", branch]);
    await run(["git", "push", "origin", tag]);
    console.log(`Pushed ${branch} and ${tag} to origin.`);
  } else {
    console.log(`\nNot pushed. When ready:\n  git push origin ${branch} && git push origin ${tag}`);
  }

  console.log(`\nRelease ${newVersion} complete.`);
}

main().catch((err) => {
  console.error("Error:", err.message);
  process.exit(1);
});
```

### 4e. `package.json`

```json
{
  "name": "<APP_NAME_LOWER>",
  "private": true,
  "scripts": {
    "release": "./Scripts/release.ts"
  }
}
```

### 4f. `.gitignore` additions

```
/build
/<APP_NAME>.app
```

(`build/` holds the release artifacts; `<APP_NAME>.app` is the signed bundle left in the project root.)

---

## 5. Make scripts executable

```bash
chmod +x Scripts/package_app.sh Scripts/release.sh Scripts/release.ts
```

---

## 6. Running a release

```bash
bun release patch --push      # 1.0.0 -> 1.0.1: bump, build, notarize, commit, tag, push
bun release minor             # local only; push manually after verifying
bun release major --push
bun release --help
```

Takes 3–5 min end-to-end (the bottleneck is Apple's notary service).

Output in `build/`:
- `<APP_NAME>-<VERSION>.zip` — signed + notarized + stapled, ready to distribute
- `<APP_NAME>-<VERSION>.dmg` — same, with drag-to-Applications layout
- `<APP_NAME>-<VERSION>.sha256` — hashes for download integrity
- `<APP_NAME>-<VERSION>-notary.log` — Apple submission audit trail

Lower-level:
- `./Scripts/package_app.sh` — local signed build without notarization (fast, for dev installs).
- `./Scripts/release.sh` — full notarization without version bump or git (re-notarize same version).
- `VERSION=1.0.0-test ./Scripts/release.sh` — throwaway test build with a one-off version.

---

## 7. Gotchas (lessons from the trenches)

### `set -o pipefail` + `grep -q` = false negative

```bash
if ! codesign -d -v "$bundle" 2>&1 | grep -q "flags=.*runtime"; then   # WRONG
```

`grep -q` exits 0 early on match and closes the pipe. `codesign`, still writing, gets SIGPIPE (exit 141). With `pipefail`, the whole pipeline exits non-zero, and `!` flips it to "true" — false failure.

**Fix**: capture output into a variable, then grep:

```bash
codesign_out=$(codesign -d -v "$bundle" 2>&1)
if ! grep -q "flags=.*runtime" <<< "$codesign_out"; then
```

### DMGs don't have a hardened runtime flag

DMGs are disk images, not executables. `codesign -d -v <dmg>` shows `flags=0x0(none)`. Don't check `flags=.*runtime` on them — gate that check on `kind == "app"` in your `verify()`.

### `notarytool submit --wait` default timeout is 3 hours

If Apple's notary service hiccups, your script hangs all afternoon. Always pass `--timeout 20m` (or similar sane value).

### Single-submission DMG stapling isn't fully guaranteed

Submitting a DMG publishes tickets for the DMG cdhash and (usually) each signed binary inside. `stapler staple <app>` retrieves the app's ticket by cdhash. In practice this works, but Apple doesn't explicitly guarantee it. If `stapler staple` on the app fails with "no ticket available", fall back to submitting a zip of the app separately and retry.

### `codesign --deep` is deprecated

Don't use it. Sign inner bundles/frameworks explicitly, inner-to-outer, before signing the outer app. SPM-only projects typically have nothing inner to sign.

### `codesign --entitlements` with an empty plist

Equivalent to omitting the flag entirely. Don't create an empty `.entitlements` file "just in case" — add it only when you actually need to declare entitlements.

### `defaults read` is finicky

Prefer `plutil -extract <KEY> raw -o - <path-to-Info.plist>`. Cleaner errors, no caching quirks.

### Submission ID audit trail

`tee` notarytool's output to a log file next to the artifact. You'll want the submission ID later if Apple flags something or you need to reconstruct history.

### Working tree must be clean before bumping VERSION

The `release.ts` preflight enforces this. Without it, a dirty tree at release time means your `git commit` picks up unrelated changes under the "release: X.Y.Z" message.

### Revert VERSION on pipeline failure

If `release.sh` fails after you've written the new version, reset `VERSION` back to the old value so the repo isn't left in a half-released state. The TS orchestrator does this in its catch block.

---

## 8. Adaptation checklist (new project)

1. Get a **Developer ID Application** cert into your login keychain.
2. Run `xcrun notarytool store-credentials <profile> --apple-id … --team-id … --password …`.
3. Copy `Scripts/package_app.sh`, `Scripts/release.sh`, `Scripts/release.ts`, `package.json`, `VERSION`, and the `.gitignore` additions from this playbook.
4. Search-and-replace placeholders in the scripts:
   - `<APP_NAME>` → your app's executable + product name
   - `<BUNDLE_ID>` → `com.yourdomain.AppName`
   - `<SIGN_IDENTITY>` → exact common name from `security find-identity -v -p codesigning`
   - `<TEAM_ID>` → your 10-char Apple team ID
   - `<NOTARY_PROFILE>` → the keychain profile name you chose in step 2
   - `<MIN_OS_VERSION>` → minimum macOS version (match your Package.swift)
5. If the app is not a menu bar app: remove the `<key>LSUIElement</key><true/>` lines from `package_app.sh`.
6. Put an icon at `Resources/AppIcon.icns` (or remove the `cp` and `CFBundleIconFile` lines).
7. `chmod +x Scripts/*.sh Scripts/*.ts`
8. Make sure `Package.swift` builds a macOS executable product whose target's name matches `<APP_NAME>` (the binary `.build/release/<APP_NAME>` is what gets copied into the bundle).
9. Test: `./Scripts/package_app.sh` → `open <APP_NAME>.app` (local sign works).
10. Test: `bun release patch` → full pipeline, no `--push` first.
11. Once working, alias `bun release patch --push` as your normal release command.

---

## 9. Reference

- [Notarizing macOS software before distribution (Apple)](https://developer.apple.com/documentation/security/notarizing_macos_software_before_distribution)
- [`xcrun notarytool`](https://developer.apple.com/documentation/security/customizing_the_notarization_workflow)
- [`codesign(1)`](x-man-page://1/codesign), [`stapler(1)`](x-man-page://1/stapler), [`spctl(8)`](x-man-page://8/spctl)
- Example implementation: this repo (Shun). Full design discussion in `docs/designs/2026-04-16-signing-and-notarization.md`.

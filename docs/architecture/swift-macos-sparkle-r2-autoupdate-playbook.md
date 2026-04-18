# Swift macOS Sparkle + R2 Auto-Update Playbook

**Purpose**: reusable recipe for wiring Sparkle 2 auto-updates into a Swift Package Manager macOS app, publishing signed/notarized artifacts plus the appcast to Cloudflare R2, and shipping a full release in one command.

**Status**: battle-tested on Shun (1.0.3). Copy to a new project, swap the placeholders, follow the setup, ship.

**What you get**:
- Your app checks for updates on a schedule and offers one-click install with EdDSA + codesign verification.
- Menu bar "Check for Updates…" + Settings toggle for automatic checks.
- Signed/notarized DMG + ZIP on R2 with stable URLs (`<app>-latest.dmg` for website downloads, versioned files for archival).
- Appcast served from R2 as the feed URL.
- One command to publish: `bun release patch --push --upload`.

**Prereq**: follow `swift-macos-release-playbook.md` first. This playbook assumes you already have Developer ID signing + notarization working.

---

## 1. Prerequisites

- macOS SPM project with `Scripts/package_app.sh`, `Scripts/release.sh`, `Scripts/release.ts` from the release playbook.
- Cloudflare account + R2 enabled (free tier works).
- Bun installed.
- One-time macOS dev setup: `brew install --cask sparkle`.

---

## 2. One-time project setup

### 2a. Create R2 bucket + token

In the Cloudflare dashboard:

1. R2 → Create bucket. Name it something shared across your apps (e.g. `downloads`).
2. Open the bucket → Settings → Public access → enable the `r2.dev` subdomain. Copy the URL (looks like `https://pub-<hex>.r2.dev`).
3. R2 → Manage API tokens → Create API token:
   - Permissions: **Object Read & Write**
   - Scope: specific bucket only
   - Copy the Access Key ID + Secret Access Key + Account ID (shown on the confirmation page — only time it's visible).

### 2b. Generate Sparkle EdDSA key

The `sparkle` Homebrew cask bundles the CLI tools but does **not** put them on PATH (casks never do). The tools live under `/opt/homebrew/Caskroom/sparkle/<ver>/bin/`:

```bash
"$(ls -d /opt/homebrew/Caskroom/sparkle/*/bin | sort | tail -1)/generate_keys"
```

This prints the **public** key to stdout and stores the **private** key in your login keychain (service: `https://sparkle-project.org`, account: `ed25519`).

**Back up the private key immediately:**

```bash
security find-generic-password -s "https://sparkle-project.org" -a "ed25519" -w
```

Copy that base64 string into a password manager with a label like `<app> Sparkle EdDSA private key`. If you lose it, you can never ship another Sparkle update to existing users — they'd have to re-download manually.

### 2c. What goes in the repo

| File | Purpose | Committed? |
|---|---|---|
| `Scripts/sparkle.config` | Public EdDSA key + feed URL + public base URL | yes |
| `Scripts/upload_r2.ts` | Bun-native R2 publish script | yes |
| `.env` | R2 credentials | **no** (gitignored at repo root) |
| `.env.example` | Credential template | yes |
| `CHANGELOG.md` | Keep-a-Changelog format | yes |
| `Sources/<App>/Services/UpdaterService.swift` | Sparkle wrapper | yes |

---

## 3. Add Sparkle to `Package.swift`

Pin the minor version — Sparkle occasionally changes delegate contracts across minor releases:

```swift
dependencies: [
    // ...existing deps...
    .package(url: "https://github.com/sparkle-project/Sparkle", .upToNextMinor(from: "2.7.0")),
],
targets: [
    .executableTarget(
        name: "YourApp",
        dependencies: [
            "Sparkle",
            // ...existing...
        ],
        // ...
    ),
]
```

Sparkle ships as a **binary XCFramework** via SPM — `swift build` downloads it to `.build/artifacts/sparkle/Sparkle/Sparkle.xcframework/macos-*/Sparkle.framework`.

---

## 4. `UpdaterService.swift`

Split the delegate from the `@Observable` facade to avoid three-way overloading (`@MainActor` + `@Observable` + `NSObject` + `SPUUpdaterDelegate`) which is fragile under Swift 6 strict concurrency.

```swift
// Sources/<App>/Services/UpdaterService.swift
import Sparkle
import os

private let log = Logger(subsystem: "com.example.YourApp", category: "updater")

// Bare delegate — NSObject, no actor isolation. Logs update errors to Console.app.
final class SparkleDelegate: NSObject, SPUUpdaterDelegate {
    func updater(_ updater: SPUUpdater, didAbortWithError error: Error) {
        log.error("Update aborted: \(error.localizedDescription, privacy: .public)")
    }

    func updater(
        _ updater: SPUUpdater,
        didFinishUpdateCycleFor updateCheck: SPUUpdateCheck,
        error: Error?
    ) {
        if let error {
            log.error(
                "Update cycle error (\(updateCheck.rawValue, privacy: .public)): \(error.localizedDescription, privacy: .public)"
            )
        }
    }
}

@MainActor
final class UpdaterService {
    private let delegate = SparkleDelegate()
    private let controller: SPUStandardUpdaterController

    init() {
        controller = SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: delegate,
            userDriverDelegate: nil
        )
    }

    func checkForUpdates() {
        controller.checkForUpdates(nil)
    }
}
```

---

## 5. Wire into `AppDelegate`

**Guard the init** — `SPUStandardUpdaterController` asserts on init if `SUFeedURL` is missing from Info.plist, which is the case for dev builds (`swift run`). The guard keeps dev builds silent:

```swift
private var updater: UpdaterService?

func applicationDidFinishLaunching(_ notification: Notification) {
    if Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil {
        updater = UpdaterService()
    }
    // ...existing setup...
    observeUpdateChecks()
}

private func observeUpdateChecks() {
    NotificationCenter.default.addObserver(
        forName: .checkForUpdates, object: nil, queue: .main
    ) { [weak self] _ in
        Task { @MainActor in
            self?.updater?.checkForUpdates()
        }
    }
}
```

Add the notification name somewhere shared:

```swift
extension Notification.Name {
    static let checkForUpdates = Notification.Name("checkForUpdates")
}
```

---

## 6. Menu bar + Settings UI

**Menu bar** — add to your `NSMenu`:

```swift
let checkUpdatesItem = NSMenuItem(
    title: "Check for Updates…",
    action: #selector(checkForUpdates),
    keyEquivalent: ""
)
checkUpdatesItem.target = self
menu.addItem(checkUpdatesItem)

// ...and the handler:
@objc private func checkForUpdates() {
    NotificationCenter.default.post(name: .checkForUpdates, object: nil)
}
```

**Settings** — bind directly to Sparkle's `UserDefaults` key. Hide the section in dev builds where `SUFeedURL` is missing:

```swift
@AppStorage("SUEnableAutomaticChecks") private var automaticallyCheckForUpdates = true

private var updatesConfigured: Bool {
    Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil
}

// In Form:
if updatesConfigured {
    Section("Updates") {
        HStack {
            Text("Current version")
            Spacer()
            Text(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?")
                .foregroundStyle(.secondary)
        }
        Toggle("Automatically check for updates", isOn: $automaticallyCheckForUpdates)
        HStack {
            Spacer()
            Button("Check for Updates…") {
                NotificationCenter.default.post(name: .checkForUpdates, object: nil)
            }
        }
    }
}
```

---

## 7. Config files

### 7a. `Scripts/sparkle.config` (committed)

```
SPARKLE_PUBLIC_KEY=<from generate_keys>
SPARKLE_FEED_URL=https://pub-<hex>.r2.dev/<app-prefix>/appcast.xml
SPARKLE_PUBLIC_BASE_URL=https://pub-<hex>.r2.dev
```

### 7b. `.env` at repo root (gitignored — add `/.env` to `.gitignore`)

**Critical**: Bun only auto-loads `.env` from the current working directory. If `release.ts` does `process.chdir(projectRoot)`, `.env` must live at repo root, not under `Scripts/`. Easy bug to miss — silent `undefined` values at upload time.

```
R2_ACCOUNT_ID=<from Cloudflare dashboard>
R2_ACCESS_KEY_ID=<from API token creation>
R2_SECRET_ACCESS_KEY=<from API token creation>
R2_BUCKET=<bucket name>
APP_KEY_PREFIX=<your-app-slug>
```

### 7c. `CHANGELOG.md` (committed)

Keep-a-Changelog format. The release script will rename `## Unreleased` → `## <version> - <UTC date>` on each release:

```markdown
# Changelog

All notable changes to YourApp are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

```

### 7d. `package.json`

Add the upload script + `marked` for rendering release notes to HTML:

```json
{
  "scripts": {
    "release": "./Scripts/release.ts",
    "upload": "./Scripts/upload_r2.ts"
  },
  "devDependencies": {
    "marked": "^12.0.0"
  }
}
```

---

## 8. Update `Scripts/package_app.sh`

Three changes stack on top of the release playbook's `package_app.sh`:

### 8a. Source `sparkle.config` and validate the key

```bash
SPARKLE_CONFIG="Scripts/sparkle.config"
source "${SPARKLE_CONFIG}"

# Ed25519 public keys are 32 bytes => 44 chars base64 ending with '='.
if ! [[ "${SPARKLE_PUBLIC_KEY}" =~ ^[A-Za-z0-9+/]{43}=$ ]]; then
    echo "FAIL: SPARKLE_PUBLIC_KEY in ${SPARKLE_CONFIG} is malformed." >&2
    exit 1
fi
```

### 8b. **Critical: patch the rpath before signing** 🔴

`swift build` produces a Mach-O **without** `@executable_path/../Frameworks` in `LC_RPATH`. Xcode adds this automatically; SPM + hand-rolled bundling does not. Without it, dyld can't find the embedded `Sparkle.framework` at launch and your app crashes immediately with "Library not loaded". This is **the** single biggest gotcha in this whole playbook.

Right after copying the built binary, before any codesign step:

```bash
cp "${BUILD_DIR}/${APP_NAME}" "${CONTENTS}/MacOS/${APP_NAME}"

install_name_tool -add_rpath "@executable_path/../Frameworks" \
                  "${CONTENTS}/MacOS/${APP_NAME}"
```

Must run before codesigning — modifying Mach-O headers invalidates any prior signature.

### 8c. Embed Sparkle.framework and sign inside-out

```bash
mkdir -p "${CONTENTS}/Frameworks"

SPARKLE_XCFRAMEWORK=$(find .build/artifacts -type d -name "Sparkle.xcframework" | head -n 1)
SPARKLE_FRAMEWORK=$(find "${SPARKLE_XCFRAMEWORK}" -type d -name "Sparkle.framework" | head -n 1)
cp -R "${SPARKLE_FRAMEWORK}" "${CONTENTS}/Frameworks/"

EMBEDDED="${CONTENTS}/Frameworks/Sparkle.framework"

# Sign nested .xpc and .app bundles first (deepest-first via sort -rz).
while IFS= read -r -d '' bundle; do
    codesign --force --options runtime --timestamp \
             --sign "${SIGN_IDENTITY}" "${bundle}"
done < <(find "${EMBEDDED}" \( -name "*.xpc" -o -name "*.app" \) -type d -print0 | sort -rz)

# Sign the Autoupdate helper executable(s).
for helper in "${EMBEDDED}/Versions/"*/Autoupdate; do
    [[ -f "${helper}" ]] || continue
    codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${helper}"
done

# Sign the framework itself.
codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${EMBEDDED}"

# Sign the main app LAST.
codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${APP_BUNDLE}"
```

### 8d. Info.plist Sparkle keys

Add to your Info.plist heredoc:

```
<key>SUFeedURL</key>
<string>${SPARKLE_FEED_URL}</string>
<key>SUPublicEDKey</key>
<string>${SPARKLE_PUBLIC_KEY}</string>
<key>SUEnableAutomaticChecks</key>
<true/>
<key>SUScheduledCheckInterval</key>
<integer>86400</integer>
<key>SUAllowsAutomaticUpdates</key>
<true/>
<key>SUEnableSystemProfiling</key>
<false/>
```

`SUEnableSystemProfiling=NO` is an explicit privacy opt-out. Sparkle defaults to NO, but pinning it here prevents accidental activation via a future default change.

**Version invariant**: Sparkle compares updates using `CFBundleVersion` (not `CFBundleShortVersionString`). Keep both equal in `package_app.sh`, or if you adopt a separate build-number scheme later, make sure `sparkle:version` in the appcast matches `CFBundleVersion` exactly — otherwise Sparkle ranks updates by the wrong number.

---

## 9. `Scripts/upload_r2.ts`

The full script is below. Key gotchas baked in:

1. **Auto-locates `sign_update`** — tries PATH first, falls back to `/opt/homebrew/Caskroom/sparkle/*/bin/`. Don't rely on the user symlinking manually.
2. **Uploads ZIP as the Sparkle enclosure** (not DMG) — convention; smaller; no mount/unmount failure modes.
3. **Uploads DMG + `<app>-latest.dmg` alias** — the alias gives your website a stable "Download" button URL.
4. **Idempotent collision check** — if a versioned key already exists with matching ETag/size, skip. Makes retry-after-partial-failure painless; `--force` stays reserved for genuine re-upload.
5. **First-release 404 fallback** — on first run, `appcast.xml` doesn't exist yet; fall back to an empty template.
6. **Verifies via public URL HEAD, not `Bun.s3.file().stat()`** — Bun's S3 client returns `size: 0` against R2 regardless of the real size. The public HEAD also doubles as the bucket-public-access smoke test.
7. **Byte-counts strings as UTF-8** — `String.length` counts UTF-16 code units; Bun uploads UTF-8 bytes. Multi-byte chars in release notes (em-dash `—`, ellipsis `…`, arrows `→`) otherwise cause false Content-Length mismatches.

See `Scripts/upload_r2.ts` in this repo for the full implementation (~340 lines). When reusing, adjust:
- Hardcoded app name in appcast `<title>`, `<link>`, `<description>`.
- R2 key layout (uses `APP_KEY_PREFIX` env for namespacing — multi-app bucket friendly).

---

## 10. Update `Scripts/release.ts`

Add `--upload` flag + CHANGELOG auto-promotion. Key logic:

```ts
// Preflight: require non-empty ## Unreleased body when --upload
if (args.upload && readUnreleasedBody(readFileSync("CHANGELOG.md", "utf8")) === "") {
  console.error("CHANGELOG.md '## Unreleased' section is empty.");
  process.exit(1);
}

// Promote "## Unreleased" -> "## <version> - <UTC date>", insert fresh Unreleased at top.
// Use UTC so the date matches appcast <pubDate> regardless of dev timezone:
//   date -u +%F  =>  2026-04-18
const utcDate = new Date().toISOString().slice(0, 10);

// Pipeline order: build -> commit VERSION+CHANGELOG+tag -> upload -> push.
// Upload BEFORE push: if upload fails, local tag intact; user fixes and retries
// upload, then pushes. Avoids publishing a tag pointing at empty R2.
```

---

## 11. Release flow

```bash
# 1. Edit CHANGELOG.md's "## Unreleased" section with release notes
# 2. Run:
bun release patch --push --upload
```

What happens:
1. Preflight: clean tree, no duplicate tag, `## Unreleased` non-empty.
2. Bump VERSION file.
3. Promote `## Unreleased` → `## <version> - <UTC date>` in CHANGELOG.md.
4. Build + Sparkle embed + sign + notarize + DMG + zip + sha256.
5. Commit (VERSION + CHANGELOG) + tag `v<version>`.
6. EdDSA-sign the ZIP, compose/fetch appcast, upload to R2.
7. `git push origin master && git push origin v<version>`.

If upload fails, the local commit + tag stay; you retry with `./Scripts/upload_r2.ts --version X.Y.Z` and push manually.

---

## 12. Smoke test before going live

Before your first `--upload` release, confirm the pipeline works without touching R2:

```bash
./Scripts/upload_r2.ts --version <current-version> --dry-run
```

This runs preflight + sign_update + fetch existing appcast + compose new `<item>`, then prints the resulting XML and planned uploads. No R2 writes. Catches:
- Config typos in `sparkle.config`.
- Missing `sign_update` binary.
- Bad `.env` loading (if `R2_*` vars are undefined, preflight fails).
- Malformed CHANGELOG parsing.

---

## 13. Pitfalls — a short field guide

| Symptom | Cause | Fix |
|---|---|---|
| App crashes at launch, no menu bar icon, dyld "Library not loaded" | Missing `@executable_path/../Frameworks` rpath | Section 8b |
| `generate_keys: command not found` after `brew install --cask sparkle` | Casks don't symlink to PATH | Use full Caskroom path (Section 2b) |
| Upload "Content-Length mismatch" on appcast but artifacts are fine on R2 | `String.length` ≠ UTF-8 byte length with multi-byte chars | `new TextEncoder().encode(xml).length` |
| Upload "Size mismatch, remote=0" | `Bun.s3.file().stat().size` returns 0 on R2 | Verify via public HEAD instead |
| `R2_ACCESS_KEY_ID` silently undefined | `.env` under `Scripts/` | Move `.env` to repo root |
| Sparkle init crash in dev build | `swift run` has no `SUFeedURL` | Guard `UpdaterService()` on `Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil` |
| User's "Check for Updates" returns "no update available" despite new release | Appcast `sparkle:version` ≠ `CFBundleVersion` | Keep `CFBundleVersion` == `CFBundleShortVersionString` |
| First release installs can't auto-update | 1.0.N users don't have Sparkle | One manual download of 1.0.(N+1); auto-updates work from there |

---

## 14. First-release manual handoff

The first release after adding Sparkle can't auto-update existing users — they don't have Sparkle in their running binary. They need to download the new version manually once. From that version forward, auto-updates work.

Plan accordingly: consider announcing the new version via your existing channel (GitHub release notes, website post, Twitter, etc.) so users know to download it.

---

## 15. URL stability warning

The `*.r2.dev` URL is rate-limited (~10 req/s per bucket) and marked "development" by Cloudflare. For low-traffic apps it's fine indefinitely. If you outgrow it, migrate via:

**Preferred**: attach a custom domain in the R2 dashboard (needs DNS managed by Cloudflare). Update `sparkle.config` and re-release — existing users still auto-update via the old `SUFeedURL` baked into their Info.plist, which you keep pointing at R2.

**Fallback**: front R2 with a tiny Cloudflare Worker at `updates.<yourdomain>.workers.dev`. Worker proxies to R2; your `SUFeedURL` permanently points at the Worker; you swap the R2 URL behind it anytime.

Either way, **never change `SUFeedURL` for an already-shipped version**. Existing users on that version can't be reached by a new URL — the URL is baked into their Info.plist.

---

## References

- Design doc with the full rationale: `docs/designs/2026-04-17-r2-upload-and-auto-update.md`
- Release signing/notarization playbook: `docs/architecture/swift-macos-release-playbook.md`
- Sparkle 2 docs: https://sparkle-project.org/documentation/
- Keep a Changelog: https://keepachangelog.com/en/1.1.0/

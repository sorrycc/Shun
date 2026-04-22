# R2 Upload & Sparkle Auto-Update

**Date:** 2026-04-17
**Status:** Implemented and shipped (1.0.3)
**Related:** `docs/designs/2026-04-16-signing-and-notarization.md`
**Revision history:**
- rev 2 (2026-04-17) — enclosure points to ZIP not DMG; handle first-release 404; use Homebrew for Sparkle tools; CHANGELOG preflight; min-macOS from Info.plist; Sparkle error logging; explicit Content-Types.
- rev 3 (2026-04-17) — idempotent upload (size/ETag match → skip); split UpdaterService into delegate + facade; move Sparkle config into committed `Scripts/sparkle.config`; dev-build guard on UpdaterService init; document `CFBundleVersion` == `CFBundleShortVersionString` invariant; auto-promote `## Unreleased` → `## <version>`; explicit `Bun.s3` region/timeout; post-upload public-URL smoke test.
- rev 4 (2026-04-18) — `.env` at repo root (not `Scripts/.env` — Bun only auto-loads from cwd); bootstrap `CHANGELOG.md` if missing; validate `SPARKLE_PUBLIC_KEY` format at package time; use UTC dates in changelog promotion; pin Sparkle minor version; specify `--dry-run` semantics.
- rev 5 (2026-04-18) — explicit `SUEnableSystemProfiling=NO` for privacy; add stable `Shun-latest.dmg` alias for website download button.
- rev 6 (2026-04-18) — correction: Homebrew cask **doesn't** put `generate_keys`/`sign_update` on PATH (only formulas do); tools live at `/opt/homebrew/Caskroom/sparkle/<ver>/bin/`. Upload script now auto-discovers the binary; one-time setup uses the explicit Caskroom path.
- rev 7 (2026-04-18) — **post-ship bug fixes from 1.0.2/1.0.3 releases:**
  - **rpath fix**: `swift build` + hand-rolled `.app` bundle does **not** get `@executable_path/../Frameworks` in `LC_RPATH` (Xcode adds it automatically, SPM does not). Without it, dyld can't find embedded `Sparkle.framework` and the app crashes at launch with "Library not loaded". `package_app.sh` now runs `install_name_tool -add_rpath` on the binary before signing.
  - **verify via public HEAD, not `Bun.s3.file().stat()`**: Bun's S3 client `stat().size` returns 0 against R2 regardless of the real size. Post-upload verification uses `fetch(url, {method: "HEAD"})` and reads `Content-Length`, which is correct and doubles as the bucket-public-access smoke test.
  - **byte-count the appcast**: `String.length` counts UTF-16 code units; Bun uploads UTF-8 bytes. Multi-byte chars in release notes (em-dash, ellipsis, arrows) tripped false Content-Length mismatches. Use `new TextEncoder().encode(xml).length` before comparing.

## Goal

Ship a complete publish pipeline for Shun releases:

1. **Upload** signed/notarized build artifacts to a shared Cloudflare R2 bucket for public download.
2. **Serve a Sparkle appcast** from R2 so the app can auto-update.
3. **Integrate Sparkle 2** in-app so running users get update prompts + one-click install.

## Non-goals

- Delta updates (binary diffs between versions). Full DMG is ~2.4 MB; not worth the complexity.
- Multi-channel feeds (beta/stable). One channel for now.
- Rollback command. If a release is bad, ship a new one.
- Delta updates, multi-channel feeds, rollback tooling — still out of scope. (Custom CDN domain is now live via `downloads-worker` — see "Custom domain" below.)

## Architecture overview

```
bun release patch --push --upload
  │
  ├─ (existing) bump VERSION → release.sh → sign/notarize/DMG/zip/sha256 → git commit + tag
  │
  ├─ (new) Scripts/upload_r2.ts
  │     1. read build/Shun-<v>.{dmg,zip,sha256}
  │     2. EdDSA-sign the ZIP via Sparkle's `sign_update` (ZIP, not DMG — Sparkle's convention)
  │     3. parse CHANGELOG.md section for <v>
  │     4. fetch + update appcast.xml (prepend new <item>; 404 → start from empty template)
  │     5. upload DMG, zip, sha256, appcast.xml to R2 via Bun.s3
  │     6. verify + print public URLs
  │
  └─ (existing) git push + git push <tag>
```

In-app: `Sources/Shun/Services/UpdaterService.swift` wraps `SPUStandardUpdaterController`. Info.plist carries `SUFeedURL` + `SUPublicEDKey`. Sparkle polls appcast daily, verifies EdDSA sig + codesign team-id on download, installs on quit.

## R2 layout

**Bucket:** `downloads` (shared across apps)
**Prefix:** `shun/` (per-app)

```
downloads/
└── shun/
    ├── appcast.xml              ← overwritten each release, Cache-Control: max-age=300
    ├── Shun-latest.dmg          ← overwritten each release (alias → current DMG), max-age=300
    ├── Shun-1.0.1.dmg           ← immutable once uploaded, Cache-Control: max-age=31536000, immutable
    ├── Shun-1.0.1.zip
    ├── Shun-1.0.1.sha256
    ├── Shun-1.0.2.dmg
    └── ...
```

**Public URLs** (custom domain, served by `~/Projects/downloads-worker` — see "Custom domain" below; `pub-<hash>.r2.dev` remains enabled as a fallback for legacy installs):
- Appcast: `https://download.sorrycc.dev/shun/appcast.xml` → baked into Info.plist as `SUFeedURL`
- Sparkle enclosure (what auto-update downloads): `https://download.sorrycc.dev/shun/Shun-1.0.1.zip`
- Versioned website download: `https://download.sorrycc.dev/shun/Shun-1.0.1.dmg`
- **Stable website "Download" button**: `https://download.sorrycc.dev/shun/Shun-latest.dmg` → always points at the current release, no website edit per release

**ZIP vs DMG split:** Sparkle convention is to ship the ZIP as the in-app update artifact (smaller, no mount/unmount, fewer failure modes). The DMG stays as the website download for manual install (familiar drag-to-Applications UX for first-time users). Both get uploaded; only the ZIP is referenced in `<enclosure>`.

**Overwrite policy:**
- Versioned files (`Shun-X.Y.Z.*`): upload fails if key exists unless `--force` passed.
- `appcast.xml`, `Shun-latest.dmg`: always overwritten (they're aliases/manifests that track the current release).

## Sparkle integration

**Dependency:** `github.com/sparkle-project/Sparkle` via SPM, alongside `KeyboardShortcuts`. **Pin the minor version** in `Package.swift` — `.upToNextMinor(from: "2.7.0")` (or whatever the latest 2.x at implementation time is). Sparkle occasionally introduces breaking changes across minor versions (delegate contracts, entitlement requirements); pinning avoids surprise CI breakage.

**New file: `Sources/Shun/Services/UpdaterService.swift`**

Split into **two types** to avoid three-way overloading (`@MainActor` + `@Observable` + `NSObject` + `SPUUpdaterDelegate`), which is fragile under Swift 6 strict concurrency and the `@Observable` macro's NSObject interaction:

```swift
import Sparkle
import os

private let log = Logger(subsystem: "com.sorrycc.shun", category: "updater")

// Bare delegate — NSObject, no actor isolation, no @Observable.
// Keeps protocol conformance completely separate from UI state.
final class SparkleDelegate: NSObject, SPUUpdaterDelegate {
    func updater(_ updater: SPUUpdater, didAbortWithError error: Error) {
        log.error("Update aborted: \(error.localizedDescription, privacy: .public)")
    }

    func updater(_ updater: SPUUpdater, didFinishUpdateCycleFor updateCheck: SPUUpdateCheck, error: Error?) {
        if let error {
            log.error("Update cycle error (\(updateCheck.rawValue)): \(error.localizedDescription, privacy: .public)")
        }
    }
}

// @Observable facade — pure Swift class, bindable from SwiftUI.
@MainActor @Observable
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

    var automaticallyChecks: Bool {
        get { controller.updater.automaticallyChecksForUpdates }
        set { controller.updater.automaticallyChecksForUpdates = newValue }
    }

    func checkForUpdates() {
        controller.checkForUpdates(nil)
    }
}
```

**Dev-build guard** in `AppDelegate.applicationDidFinishLaunching`:

```swift
// Only create the updater when packaged — dev builds (swift run) have no SUFeedURL
// in Info.plist and would spam Sparkle init warnings.
if Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil {
    self.updater = UpdaterService()
}
```

Errors stream to Console.app under subsystem `com.sorrycc.shun`, category `updater`.

**Info.plist keys** (set by `Scripts/package_app.sh`):

| Key | Value | Notes |
|---|---|---|
| `SUFeedURL` | `https://download.sorrycc.dev/shun/appcast.xml` | from `sparkle.config` |
| `SUPublicEDKey` | `<base64 public key>` | from `sparkle.config` |
| `SUEnableAutomaticChecks` | `YES` | |
| `SUScheduledCheckInterval` | `86400` | 1 day |
| `SUAllowsAutomaticUpdates` | `YES` | |
| `SUEnableSystemProfiling` | `NO` | **explicit privacy opt-out** — don't send OS/CPU profile with update checks |

`LSUIElement=YES` is already set — Sparkle's default `SPUStandardUserDriver` supports menu-bar apps (dialog appears in foreground when triggered).

**Version invariant:** Sparkle compares updates using `CFBundleVersion` (not `CFBundleShortVersionString`). `package_app.sh` **must** set both to the same value (or if they diverge later via a build-number scheme, `sparkle:version` in the appcast must match `CFBundleVersion` exactly). If this invariant breaks, Sparkle ranks updates by the wrong number and either skips or loops on the same version. Documented here so future build-number refactors don't silently break updates.

**Critical rpath patch in `package_app.sh`:** `swift build` produces a Mach-O without `@executable_path/../Frameworks` in `LC_RPATH`. Xcode adds this automatically; SPM + hand-rolled bundling does not. Without it, dyld cannot find the embedded `Sparkle.framework` at launch and the app crashes immediately with "Library not loaded". `package_app.sh` **must** patch the binary before codesigning:

```bash
install_name_tool -add_rpath "@executable_path/../Frameworks" \
                  "${CONTENTS}/MacOS/${APP_NAME}"
```

This runs right after copying the binary, before any codesign step (modifying Mach-O headers invalidates signatures). **This caused 1.0.2 to crash on every launch**; 1.0.3 added the fix.

**UI additions:**

- `Views/SettingsView.swift` General tab:
  - Button "Check for Updates…" → `updater.checkForUpdates()`
  - Toggle "Automatically check for updates" bound to `updater.automaticallyChecks`
  - Read-only "Current version: 1.0.1" from `Info.plist`
- `StatusItemController.swift` NSMenu:
  - "Check for Updates…" item → posts `.checkForUpdates` Notification → AppDelegate handles

**Key management:**

- Install Sparkle tools via Homebrew: `brew install --cask sparkle`
  - Gives stable paths: `generate_keys` and `sign_update` in the cask's `bin/`
  - Avoids fragile `.build/checkouts/Sparkle/bin/*` paths (vanish on `swift package clean`)
- One-time: run `generate_keys`
  - Private EdDSA key → stored in login keychain (Sparkle manages this)
  - Public key printed to stdout → pasted into `Scripts/sparkle.config` (see below)
- Private key must be backed up manually (lose it = can never ship another update for existing users).

**New committed config file: `Scripts/sparkle.config`** — single source of truth for the public key and feed URL, consumed by **both** `package_app.sh` (writes Info.plist) and `upload_r2.ts` (knows the public URL):

```
SPARKLE_PUBLIC_KEY=<base64 EdDSA public key>
SPARKLE_FEED_URL=https://download.sorrycc.dev/shun/appcast.xml
SPARKLE_PUBLIC_BASE_URL=https://download.sorrycc.dev
```

`package_app.sh` sources this file and substitutes into the generated Info.plist. Rotating the EdDSA key or migrating the feed URL becomes a one-line diff instead of an edit scattered across a shell script and a TS file.

**Key validation at package time:** `package_app.sh` must validate `SPARKLE_PUBLIC_KEY` before writing it into Info.plist — Ed25519 public keys are 32 bytes, so base64 form matches `^[A-Za-z0-9+/]{43}=$` (44 chars total). Bail out loudly if empty or malformed. Catches the "typo in config" failure mode at build time instead of at a user's runtime (where a bad key silently disables all updates).

## Upload script (`Scripts/upload_r2.ts`)

~150 lines, native Bun + `Bun.s3`, no extra runtime deps.

**CLI:**

```
./Scripts/upload_r2.ts                 # uploads build/ artifacts for current VERSION
./Scripts/upload_r2.ts --version 1.0.1 # explicit version
./Scripts/upload_r2.ts --dry-run       # see below
./Scripts/upload_r2.ts --force         # overwrite versioned artifacts (use with care)
```

**`--dry-run` semantics:** runs preflight + sign_update + fetch existing appcast + compose new `<item>`, then prints the resulting appcast XML and the list of planned uploads (key + size + content-type + target URL). Skips collision check, uploads, HEAD verifies, and public URL smoke test. Lets you validate config and CHANGELOG parsing without touching R2.

**Config (`.env` at repo root, gitignored — secrets only):**

```
R2_ACCOUNT_ID=<cloudflare account id>
R2_ACCESS_KEY_ID=<token access key>
R2_SECRET_ACCESS_KEY=<token secret>
R2_BUCKET=downloads
APP_KEY_PREFIX=shun
```

**Location matters:** Bun only auto-loads `.env` from the current working directory. Since `release.ts` does `process.chdir(projectRoot)`, the file **must** live at repo root (`./.env`), not `./Scripts/.env`. If you put it under `Scripts/`, `Bun.env.R2_*` will be `undefined` and the script will fail with a cryptic auth error from R2. Gitignore rule is `/.env` at the root.

The public URL is **not** here — it comes from `Scripts/sparkle.config` (committed), so the upload script and Info.plist agree by construction.

**S3 client config** (inside `upload_r2.ts`):

```ts
const s3 = new Bun.S3Client({
  accessKeyId: Bun.env.R2_ACCESS_KEY_ID!,
  secretAccessKey: Bun.env.R2_SECRET_ACCESS_KEY!,
  endpoint: `https://${Bun.env.R2_ACCOUNT_ID}.r2.cloudflarestorage.com`,
  region: "auto",          // R2 requires an explicit region string for SigV4
  bucket: Bun.env.R2_BUCKET!,
  // No built-in per-request timeout in Bun.s3 today; wrap each upload in Promise.race
  // with a 60s timeout and retry 3x with exponential backoff for flaky networks.
});
```

**Flow:**

1. **Preflight.** Verify env vars present; verify `build/Shun-<v>.{dmg,zip,sha256}` exist; verify `sign_update` is on PATH (fail early with "run `brew install --cask sparkle`").
2. **EdDSA-sign the ZIP.** Shell out to `sign_update build/Shun-<v>.zip` (Sparkle convention — ZIP, not DMG). Produces `sparkle:edSignature="…" length="…"`. Signing key pulled from keychain automatically.
3. **Read `LSMinimumSystemVersion`** from `Shun.app/Contents/Info.plist` via `plutil -extract LSMinimumSystemVersion raw` — single source of truth, avoids drift when bumping macOS support.
4. **Fetch existing appcast.xml** from R2. **On 404**, start from an empty template (first release). On other errors, abort. Parse existing `<item>` entries to preserve history.
5. **Build new `<item>`** from a tiny template (no XML library — Sparkle schema is stable and small):
   - `<title>Version 1.0.1</title>`
   - `<pubDate>RFC-822 now</pubDate>`
   - `<description><![CDATA[ <html from CHANGELOG.md> ]]></description>`
   - `<sparkle:minimumSystemVersion>{value from step 3}</sparkle:minimumSystemVersion>`
   - `<enclosure url="…/Shun-1.0.1.zip" sparkle:version="1.0.1" sparkle:shortVersionString="1.0.1" length="…" type="application/zip" sparkle:edSignature="…"/>`
6. **Idempotent collision check.** HEAD each versioned key. For each:
   - **Key missing** → upload as usual.
   - **Key present, Content-Length matches local, ETag matches local MD5** → skip (idempotent retry).
   - **Key present, bytes differ** → fail unless `--force` (genuine content conflict — someone re-built 1.0.1 with different artifacts).

   This makes retry after a mid-run failure safe by default — no need for `--force` just because the DMG uploaded but the appcast didn't.
7. **Upload** via the `Bun.S3Client` configured above. Correct `Content-Type` per artifact:
   - `Shun-<v>.dmg` → `application/x-apple-diskimage`, `Cache-Control: public, max-age=31536000, immutable`
   - `Shun-<v>.zip` → `application/zip`, `Cache-Control: public, max-age=31536000, immutable`
   - `Shun-<v>.sha256` → `text/plain`, `Cache-Control: public, max-age=31536000, immutable`
   - `Shun-latest.dmg` → `application/x-apple-diskimage`, `Cache-Control: public, max-age=300` (alias, copied from the same bytes as `Shun-<v>.dmg`)
   - `appcast.xml` → `application/xml`, `Cache-Control: public, max-age=300`
   - DMG/zip/sha256 upload in parallel first; **then** `Shun-latest.dmg` + `appcast.xml` last (so neither alias ever references artifacts that aren't yet live).
8. **Verify via public URL HEAD (single round trip).** `fetch(url, {method: "HEAD"})` for every uploaded key, compare against `Content-Length`. Two reasons to prefer this over `Bun.s3.file(key).stat()`:
   - Bun's S3 client returns `size: 0` from `.stat()` against Cloudflare R2 (confirmed empirically; bug or feature of the current Bun.s3 API — don't trust it).
   - Public-URL HEAD doubles as the bucket-public-access smoke test, catching the silent-misconfig case where R2 accepts writes but users can't download.
   For the appcast, compute expected size as `new TextEncoder().encode(xml).length`, **not** `xml.length` — `String.length` counts UTF-16 code units, but Bun uploads UTF-8 bytes. Multi-byte characters in release notes (em-dash `—`, ellipsis `…`, arrows `→`) otherwise cause false Content-Length mismatches.
9. **Print public URLs.**

**CHANGELOG.md convention — Keep-a-Changelog with auto-promotion:**

The changelog always has an `## Unreleased` section at the top that's edited as you work. On `bun release`, the release script:
1. **Bootstrap** if `CHANGELOG.md` doesn't exist → write a Keep-a-Changelog template with an empty `## Unreleased` section. Fail preflight (empty body) — forces the user to describe the release before proceeding.
2. Verifies `## Unreleased` exists and has a non-empty body (preflight gate — see Release flow).
3. Renames `## Unreleased` → `## <new-version> - <YYYY-MM-DD>` using **UTC** (`date -u +%F`) so the date is unambiguous across timezones and matches the `<pubDate>` in the appcast.
4. Inserts a fresh empty `## Unreleased` section at the top.
5. Commits the changelog edit as part of the release commit.

Upload script parses `## <new-version>` (now present) and converts the body to HTML via `marked` (dev dep, tiny, replaceable). Removes the entire "I forgot to add a changelog entry for 1.0.2 before releasing" class of failures.

**Error handling:**

- Upload failure mid-run → retry is safe: the new idempotent collision check skips already-uploaded artifacts when bytes match; appcast overwrite is idempotent.
- Genuine content conflict (bytes differ for an existing versioned key) → fails without `--force`. `--force` stays reserved for the rare "I need to replace a published artifact" case and should be accompanied by appcast surgery.
- Missing signing key → `sign_update` exits non-zero; surface clearly with pointer to one-time setup docs.
- Public URL HEAD fails but S3 HEAD succeeded → bucket public access is misconfigured; print exact dashboard path to enable it.

## Release flow integration

**Modify `Scripts/release.ts`:** add `--upload` flag, call `upload_r2.ts` between git-tag and git-push.

```
bun release patch                   # local-only: bump, build, commit, tag. (unchanged)
bun release patch --push            # also push commit + tag. (unchanged)
bun release patch --push --upload   # ALSO publish to R2.
bun release patch --upload          # upload without pushing.
```

**Pipeline order:**

1. Preflight: clean tree, no duplicate tag (unchanged)
2. **NEW preflight:** if `--upload`, verify `CHANGELOG.md` has an `## Unreleased` section with a non-empty body. Fail before any build work.
3. Bump VERSION (unchanged)
4. **NEW:** promote `## Unreleased` → `## <new-version> - <date>` in `CHANGELOG.md`, insert fresh empty `## Unreleased` at top.
5. `release.sh` → artifacts (unchanged)
6. git commit (**now includes VERSION + CHANGELOG.md**) + git tag (otherwise unchanged)
7. **NEW:** if `--upload`, run `upload_r2.ts`
8. if `--push`, `git push` + `git push <tag>` (unchanged)

**Rationale — upload before push:** If upload fails, the local tag is preserved, user fixes + retries upload, then pushes. If push happened first and upload failed, users would see a published tag on GitHub with no downloadable binary.

**Failure handling:** upload failure leaves local commit + tag intact, skips push, exits non-zero. No auto-rollback of VERSION — the tagged commit is valid, just not published.

## Files changed

- `Scripts/upload_r2.ts` (new)
- `Scripts/release.ts` (add `--upload`, CHANGELOG preflight + auto-promotion, stage CHANGELOG.md in commit)
- `.env` (new, gitignored, **at repo root** — Bun's auto-load cwd)
- `.env.example` (new, committed, at repo root)
- `Scripts/sparkle.config` (new, committed — public key + feed URL)
- `Scripts/package_app.sh` (source `sparkle.config`, write SU* keys + ensure `CFBundleVersion` == `CFBundleShortVersionString`)
- `.gitignore` (add `/.env`)
- `package.json` (add `upload` script + `marked` devDependency)
- `Package.swift` (add Sparkle dep)
- `Sources/Shun/Services/UpdaterService.swift` (new — delegate + facade split)
- `Sources/Shun/AppDelegate.swift` (guarded UpdaterService init — only when `SUFeedURL` present)
- `Sources/Shun/Views/SettingsView.swift` (check-for-updates UI)
- `Sources/Shun/StatusItemController.swift` (menu item + notification)
- `CHANGELOG.md` (new — Keep-a-Changelog format with `## Unreleased`)
- `CLAUDE.md` (document `--upload` flag + one-time setup)

## One-time setup

1. **Create R2 bucket**
   - Cloudflare dashboard → R2 → create bucket `downloads`
   - Settings → Public access → enable `r2.dev` subdomain (kept on for legacy-install fallback; not the primary public host)
2. **Deploy downloads-worker** (`~/Projects/downloads-worker`)
   - Cloudflare Worker with an R2 binding to `downloads` and a custom-domain route on `download.sorrycc.dev`. `bun run deploy` provisions DNS + cert.
3. **Create R2 API token**
   - R2 → Manage API tokens → Object Read & Write, scoped to `downloads`
   - Put Access Key ID + Secret in `./.env` at repo root (not under `Scripts/`)
4. **Install Sparkle tools**
   - `brew install --cask sparkle` — provides `generate_keys` and `sign_update` on PATH
5. **Generate Sparkle EdDSA key**
   - Run `generate_keys` (from the cask)
   - Paste the printed public key into `Scripts/sparkle.config` as `SPARKLE_PUBLIC_KEY=…`
   - **Back up the private key** (export from keychain → password manager)
6. **Set feed URL** in `Scripts/sparkle.config` — `SPARKLE_FEED_URL=https://download.sorrycc.dev/shun/appcast.xml` and `SPARKLE_PUBLIC_BASE_URL=https://download.sorrycc.dev`. Commit the file.

## Smoke test

1. `VERSION=1.0.2-test ./Scripts/release.sh && ./Scripts/upload_r2.ts --version 1.0.2-test`
2. `curl https://download.sorrycc.dev/shun/appcast.xml` — confirm new `<item>`
3. Install previous (1.0.1) build locally, launch → update prompt appears
4. Accept → Sparkle downloads + verifies + relaunches
5. Delete test keys from R2, revert VERSION

## Risks & caveats

- **r2.dev rate limit** (~10 req/s per bucket) — no longer on the critical path; traffic goes through `download.sorrycc.dev` (Worker + R2 binding) which removes that ceiling. r2.dev stays enabled so shipped 1.0.4/1.0.5 installs keep receiving updates.
- **SUFeedURL is permanent.** Changing it post-release strands old users. Worker redirect layer is cheap insurance if we ever move DNS to Cloudflare.
- **Public EdDSA key is permanent.** Losing the private key = no more updates for existing users.
- **First post-integration release** (1.0.2) can't auto-update 1.0.1 users — they lack Sparkle. Manual download required; 1.0.2 → 1.0.3+ auto-updates normally.
- **Appcast is append-only in practice.** Once a version's `<item>` is published, removing it requires careful thought (users mid-update may fail). Upload script never deletes entries, only prepends.

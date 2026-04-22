# Fix KeyboardShortcuts resource-bundle crash in packaged .app

## 1. Background

GitHub issue [#1](https://github.com/sorrycc/shun/issues/1): users report a hard
crash when opening the Shortcut Recorder in the packaged `.app`, after which
the app fails to relaunch. Reproduced on two machines running the notarized
1.0.4 DMG. Crash log:

```
KeyboardShortcuts/resource_bundle_accessor.swift:12: Fatal error:
could not load resource bundle:
  from /Applications/Shun.app/KeyboardShortcuts_KeyboardShortcuts.bundle
    or /Users/chencheng/Projects/shun/.build/arm64-apple-macosx/release/KeyboardShortcuts_KeyboardShortcuts.bundle
```

Same class of bug as commit `2de3cca` (the menu-bar-icon `Bundle.module` fix),
but for a third-party dependency (KeyboardShortcuts) whose accessor we don't
own. The dev machine masks this because the hardcoded `.build/...` fallback
path exists locally.

## 2. Requirements Summary

**Goal:** Shortcut Recorder must not crash on user machines. The packaged
`.app` must satisfy the path that `Bundle.module` resolves to at runtime, and
must still pass `codesign --verify --strict` + Gatekeeper/notarization.

**Scope:**
- In scope: `Scripts/package_app.sh` — build pipeline patches + embeds +
  signs the bundle.
- Out of scope: Modifying our `Package.swift` (the SPM-resources caveat in
  `CLAUDE.md` applies to our target; this fix affects a dependency's bundle).
- Out of scope: Forking `sindresorhus/KeyboardShortcuts` (evaluated, rejected
  for maintenance cost).

**Key facts grounded in code:**
- `.build/arm64-apple-macosx/release/KeyboardShortcuts.build/DerivedSources/resource_bundle_accessor.swift`
  line 5: `Bundle.main.bundleURL.appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle").path`.
- For a packaged `.app`, `Bundle.main.bundleURL` is the `.app` root. The crash
  log confirms it searches at `/Applications/Shun.app/...`.
- SwiftPM emits the built bundle at
  `.build/release/KeyboardShortcuts_KeyboardShortcuts.bundle` — a flat bundle
  (`Info.plist` + 14 `*.lproj/` dirs, no executable code).
- Empirically verified: **`codesign` refuses to sign an `.app` with content
  at the bundle root**, emitting `unsealed contents present in the bundle
  root` and exiting non-zero, even with `--deep`. So the bundle cannot live at
  the `.app` root, even though that's where the accessor looks.

## 3. Acceptance Criteria

1. After `./Scripts/package_app.sh`, `Shun.app/Contents/Resources/KeyboardShortcuts_KeyboardShortcuts.bundle/`
   exists with `Info.plist` and all `*.lproj/` subdirectories.
2. The `.app` root contains only `Contents/`. The embedded bundle is
   Developer-ID signed (timestamped) and ends up under `Contents/Resources/`
   — no content at the bundle root.
3. `codesign --verify --strict --verbose=2 Shun.app` exits 0. `Shun.app: valid
   on disk` and `Shun.app: satisfies its Designated Requirement`.
4. Shortcut Recorder opens without crashing on a machine with no `.build/`
   at the hardcoded fallback path. Relaunching after opening the Recorder
   works cleanly.

## 4. Problem Analysis

**What exists today.** `Scripts/package_app.sh` copies the executable, icons,
and `Sparkle.framework` into `Contents/`, then signs inside-out. It does not
copy any SwiftPM-generated resource bundles from dependencies. At runtime,
KeyboardShortcuts calls `Bundle.module` which fails the primary lookup
(`.app/KeyboardShortcuts_KeyboardShortcuts.bundle`), falls back to the
compile-time `.build/...` absolute path, doesn't find anything on a user
machine, and hits `fatalError`.

**Approaches evaluated:**

- **Copy bundle to `Contents/Resources/`** (as suggested in the issue body).
  Rejected initially because the accessor uses `Bundle.main.bundleURL`, not
  `.resourceURL`. On its own, this fixes codesign but not the crash.
- **Copy bundle to the `.app` root.** This matches the accessor's path at
  runtime but breaks codesign: `Shun.app: unsealed contents present in the
  bundle root`, exit 1. Tested with and without `--deep`. Not viable.
- **Runtime swizzle `-[NSBundle initWithPath:]`** to redirect the problematic
  lookup. Works in principle but (a) intrusive — intercepts every `Bundle(path:)`
  call process-wide, (b) fragile under future Foundation changes, (c) must
  run before any KeyboardShortcuts code.
- **Fork the dependency.** Robust but introduces GH availability + manual
  upstream-sync burden for a well-maintained upstream.
- **Binary-patch the compiled Mach-O** to shorten the hardcoded `buildPath`
  string to a shared location (e.g., `/tmp/...`). Fragile, string-length
  constraints, runtime file-copy dance required.
- **Chosen: patch the generated accessor at build time + put bundle under
  `Contents/Resources/`.** SwiftPM regenerates `resource_bundle_accessor.swift`
  on every build, but we can pin a patched version by making the file
  read-only (`chmod 444`) between build passes. A small `sed` tweak changes
  `Bundle.main.bundleURL` to `(Bundle.main.resourceURL ?? Bundle.main.bundleURL)`,
  which for a packaged `.app` resolves to `Contents/Resources/` (codesign-safe)
  and for `swift run` still resolves to the executable's directory (unchanged).

## 5. Decision Log

**1. Where to place the bundle inside the `.app`?**
- Options: A) `Contents/Resources/` · B) `.app` root
- Decision: **A)** — `.app` root fails `codesign` (empirically verified).
  `Contents/Resources/` is the only codesign-safe location for a nested bundle.

**2. How to make the accessor find the bundle at `Contents/Resources/`?**
- Options: A) Patch the generated accessor · B) Fork the dependency ·
  C) Runtime swizzle `Bundle.init(path:)` · D) Binary-patch the Mach-O
- Decision: **A)** — simplest, most self-contained, no long-term maintenance
  burden, no third-party fork, no runtime-global side effects.

**3. How to prevent SwiftPM from overwriting the patch on the next build?**
- Options: A) `chmod 444` the patched file · B) Delete SPM's build DB ·
  C) Custom SPM build plugin
- Decision: **A)** — verified: SPM on an incremental build sees the file is
  present and considered up-to-date, so it skips regeneration; `chmod 444`
  additionally guarantees that even if it *tries* to overwrite, it can't.
  Restore `chmod 644` at the end so dev `swift build` keeps working.

**4. Build-pass sequence?**
- Options: A) 1 pass (patch before first build — fails: file doesn't exist yet) ·
  B) 2 passes (pass 1 generates; patch + chmod 444; pass 2 fails with `invalid
  access` on fresh `.build/` because SPM still tries to re-emit) · C) 3 passes
  (pass 1 generates; pass 2 is an idle pass that settles SPM's incremental
  state; patch + chmod 444; pass 3 compiles with patched source).
- Decision: **C)** — empirically verified as the only reliable sequence.
  Pass 2 is a SPM no-op (~0.2 s) that avoids the `invalid access` error by
  putting SPM's build DB into a state where DerivedSources are considered
  up-to-date.

**5. Sign the embedded bundle or not?**
- Options: A) Unsigned · B) Sign with `--timestamp` only · C) Sign with
  `--options runtime --timestamp`
- Decision: **B)** — nested bundles under a hardened-runtime app should carry
  their own Developer-ID signature for notarization robustness, but
  `--options runtime` is a property of executables; the KeyboardShortcuts
  bundle contains only `.strings`, no Mach-O. Correct flags: `--timestamp --sign`.

**6. Restore `chmod 644` after packaging completes?**
- Options: A) Leave `chmod 444` · B) Restore `chmod 644`
- Decision: **B)** — otherwise a subsequent `swift build` (dev workflow,
  `swift run`) fails with `invalid access`. Restoring writability lets SPM
  regenerate from its template on the next dev build. The next
  `package_app.sh` run re-patches idempotently.

## 6. Design

Three build passes plus a source patch, added to `Scripts/package_app.sh`:

```sh
ACCESSOR=".build/arm64-apple-macosx/release/KeyboardShortcuts.build/DerivedSources/resource_bundle_accessor.swift"

echo "Building ${APP_NAME} ${VERSION} (release, pass 1/3)..."
swift build -c release

echo "Building ${APP_NAME} ${VERSION} (release, pass 2/3 — settle SPM incremental state)..."
swift build -c release

if [[ ! -f "${ACCESSOR}" ]]; then
    echo "FAIL: ${ACCESSOR} not found after pass 2." >&2
    exit 1
fi

if ! grep -q "Bundle\.main\.resourceURL" "${ACCESSOR}"; then
    echo "Patching KeyboardShortcuts resource_bundle_accessor.swift..."
    chmod 644 "${ACCESSOR}"
    sed -i.bak 's|Bundle\.main\.bundleURL\.appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts\.bundle")\.path|(Bundle.main.resourceURL ?? Bundle.main.bundleURL).appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle").path|' "${ACCESSOR}"
    rm "${ACCESSOR}.bak"
fi
chmod 444 "${ACCESSOR}"

echo "Building ${APP_NAME} ${VERSION} (release, pass 3/3 — with patched accessor)..."
swift build -c release

chmod 644 "${ACCESSOR}"
```

Plus: after the `.app` skeleton is assembled, copy the bundle into
`Contents/Resources/`:

```sh
cp -R "${BUILD_DIR}/KeyboardShortcuts_KeyboardShortcuts.bundle" \
      "${CONTENTS}/Resources/"
```

And sign it inside-out, before the outer-app `codesign`:

```sh
# Resource-only bundle (no Mach-O) — no --options runtime.
codesign --force --timestamp \
         --sign "${SIGN_IDENTITY}" \
         "${CONTENTS}/Resources/KeyboardShortcuts_KeyboardShortcuts.bundle"
```

### What the patch does

Original generated accessor:

```swift
let mainPath = Bundle.main.bundleURL
    .appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle").path
```

Patched:

```swift
let mainPath = (Bundle.main.resourceURL ?? Bundle.main.bundleURL)
    .appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle").path
```

For a packaged `.app`: `Bundle.main.resourceURL` = `Contents/Resources/`, so
the lookup resolves inside the codesign-sealed area. For `swift run`:
`Bundle.main.resourceURL` equals `Bundle.main.bundleURL` (both are the
executable's directory, which is where SPM placed the bundle) — unchanged.

### Why three passes

- **Pass 1 (fresh)** generates `resource_bundle_accessor.swift` from SPM's
  template. Full compile.
- **Pass 2 (idle)** is a no-op build that settles SPM's incremental state.
  Without this, pass 3 fails with `invalid access` because SPM tries to
  re-emit DerivedSources and hits the `chmod 444`.
- **Pass 3 (patched)** compiles with our patched accessor. SPM sees the file
  as up-to-date (its regen logic is keyed off of inputs that haven't changed
  since pass 1) and doesn't attempt to rewrite it.

### Why `chmod 444` at build time and `chmod 644` after

- `chmod 444` is the belt-and-suspenders safeguard: even if SPM decides to
  re-emit, it can't. The compile still succeeds because the compiler only
  needs read access.
- `chmod 644` at the end lets developers run `swift build` / `swift run`
  afterwards without seeing `invalid access`. On the next `swift build`, SPM
  regenerates from template (reverting the patch); the next `package_app.sh`
  run re-patches idempotently thanks to the `grep -q` guard.

### Data flow

Build → 3-pass `swift build -c release` with accessor patched between passes
2 and 3 → `.build/release/Shun` binary has the patched accessor baked in →
package script copies binary to `Contents/MacOS/`, bundle to `Contents/Resources/`
→ signs bundle inside-out, then outer app → outer `codesign --verify --strict`
succeeds (all content under `Contents/`) → `Scripts/release.sh` notarizes.
Runtime: `Bundle.module` in KeyboardShortcuts resolves via
`Bundle.main.resourceURL` → hits the embedded bundle in `Contents/Resources/`
→ no `fatalError`.

### Failure modes + error handling

- `ACCESSOR` file missing after pass 2: explicit `test -f` + diagnostic +
  `exit 1`, matching the existing Sparkle-check pattern.
- Pass 1 / pass 2 / pass 3 build failures: `set -euo pipefail` already propagates.
- `sed` failures (e.g., pattern no longer matches a future KeyboardShortcuts
  version): compile at pass 3 will still succeed because the file is valid
  Swift; the binary will just have the old `Bundle.main.bundleURL` behavior.
  A future version bump of `KeyboardShortcuts` in `Package.swift` should be
  followed by a local `package_app.sh` run and a manual Recorder check.
- Re-run on an already-patched tree: the `grep -q "Bundle\.main\.resourceURL"`
  guard skips the sed step. `chmod 444` re-applied idempotently.

## 7. Files Changed

- `Scripts/package_app.sh` — three-pass build with accessor patching;
  embed + sign `KeyboardShortcuts_KeyboardShortcuts.bundle` under
  `Contents/Resources/`.
- `CHANGELOG.md` — fix bullet under `## Unreleased`.

## 8. Verification

1. [AC1] `ls Shun.app/Contents/Resources/KeyboardShortcuts_KeyboardShortcuts.bundle/`
   shows `Info.plist`, `_CodeSignature/`, and all `*.lproj/` directories.
   ✓ Verified.
2. [AC2] `ls Shun.app/` shows `Contents` only (nothing unsealed at root).
   `codesign -dv Shun.app/Contents/Resources/KeyboardShortcuts_KeyboardShortcuts.bundle`
   reports `Authority=Developer ID Application: chen cheng (KU8S35TEW8)` and a
   timestamp. ✓ Verified.
3. [AC3] `codesign --verify --strict --verbose=2 Shun.app` → `Shun.app: valid
   on disk` + `Shun.app: satisfies its Designated Requirement`, exit 0.
   ✓ Verified.
4. [AC4] Install `Shun.app` to `/Applications/`, launch, open Settings —
   app stays running, no `fatalError`, no crash log entries. ✓ Launch verified.
   **Manual final check** (user action): add a shortcut, click the Recorder,
   confirm no crash. Canonical clean-machine repro: second macOS user
   account or fresh VM with no `.build/` at the hardcoded fallback path.

# SPM Resource Bundle Fix — Drop `Bundle.module` for Menu Bar Icons

Date: 2026-04-20

## Problem

On macOS 15.7.3, Shun 1.x crashes at launch:

```
Shun/resource_bundle_accessor.swift:12: Fatal error: could not load resource bundle:
  from /Applications/Shun.app/Shun_Shun.bundle
  or   /Users/chencheng/Projects/shun/.build/arm64-apple-macosx/release/Shun_Shun.bundle
```

### Root cause

`Package.swift` declares the Shun target with:

```swift
resources: [.process("Resources")],
```

SwiftPM therefore generates `Shun_Shun.bundle` (containing the 3 `MenuBarIcon*.png` files) and an auto-generated accessor at `.build/.../Shun.build/DerivedSources/resource_bundle_accessor.swift`:

```swift
let mainPath  = Bundle.main.bundleURL.appendingPathComponent("Shun_Shun.bundle").path
let buildPath = "/Users/chencheng/Projects/shun/.build/arm64-apple-macosx/release/Shun_Shun.bundle"
```

For an `.app`, `Bundle.main.bundleURL` resolves to the `.app` directory itself — so `mainPath` is `/Applications/Shun.app/Shun_Shun.bundle` (at the `.app` root, *not* `Contents/Resources/`). `Scripts/package_app.sh` (lines 40–47) never copies the SPM-generated bundle into the `.app`, so the first lookup fails; the fallback `buildPath` is a hardcoded dev-machine path that does not exist on end-user machines. Result: `fatalError` from `Bundle.module`.

The only consumer of `Bundle.module` is `StatusItemController.loadMenuBarIcon()` (Sources/Shun/StatusItemController.swift:90–109), which loads 3 PNGs for the menu bar icon.

### Why this wasn't caught in dev

On the developer's own machine, the accessor's hardcoded `buildPath` (`/Users/chencheng/Projects/shun/.build/...`) points to a real, valid `Shun_Shun.bundle` directory — so `Bundle(path: buildPath)` returns non-nil and `Bundle.module` silently "works" even when launching `/Applications/Shun.app` built via `package_app.sh`. The crash only reproduces on a machine *without* that dev path: another user account, another Mac, or after `rm -rf .build`. This is why the bug shipped: post-package smoke tests on the build machine never exercised the failure mode.

Regression-test implication: validation below explicitly requires a clean environment (fresh user account or `.build` removed) before opening `/Applications/Shun.app`.

## Decision

Stop using SwiftPM's resource system entirely. Put the menu bar icon PNGs directly in `Contents/Resources/` (the conventional macOS location) via `package_app.sh`, and load them through `Bundle.main` / `NSImage(named:)`.

Alternatives considered:

- **Copy `Shun_Shun.bundle` to `.app` root in `package_app.sh`.** Works, but places resources in a non-standard location and keeps an SPM accessor quirk in the critical path.
- **Keep PNGs under `Sources/Shun/Resources/` and `exclude:` them in `Package.swift`.** Semantically odd (files under `Sources/` not participating in compilation), and splits the packaging source paths between `Sources/Shun/Resources/` and `Resources/`.

The chosen approach matches the existing `Resources/AppIcon.icns` convention and removes a moving part (no more generated accessor, no more module bundle).

## Changes

### 1. `Package.swift`

Drop the `resources:` declaration:

```diff
 .executableTarget(
     name: "Shun",
     dependencies: [
         "KeyboardShortcuts",
         "Sparkle",
     ],
     path: "Sources/Shun",
-    resources: [
-        .process("Resources"),
-    ],
     swiftSettings: [
         .swiftLanguageMode(.v6),
     ]
 ),
```

### 2. Move the PNGs

```
git mv Sources/Shun/Resources/MenuBarIcon.png     Resources/
git mv Sources/Shun/Resources/MenuBarIcon@2x.png  Resources/
git mv Sources/Shun/Resources/MenuBarIcon@3x.png  Resources/
rmdir Sources/Shun/Resources
```

They sit alongside `Resources/AppIcon.icns` — same "packaging inputs" directory that `package_app.sh` already reads from.

### 3. `Sources/Shun/StatusItemController.swift`

Replace the manual multi-rep loop with `NSImage(named:)`, which lets AppKit pick the right `@1x/@2x/@3x` rep from `Contents/Resources/` automatically:

```swift
private static func loadMenuBarIcon() -> NSImage {
    let image = NSImage(named: "MenuBarIcon") ?? NSImage()
    image.isTemplate = true
    image.accessibilityDescription = "Shun"
    return image
}
```

### 4. `CLAUDE.md` — guardrail against regression

Add a note under the Critical Conventions section so a future change (by a human or by Claude) doesn't silently reintroduce the same bug:

> **No SPM resources on the Shun target.** Do not add `resources:` to the Shun target in `Package.swift`. The SPM-generated accessor assumes a resource-bundle layout that doesn't match a hand-packaged `.app` (and the dev machine's `.build` path masks the failure), so it would ship broken. Put additional resources in the repo-root `Resources/` directory and copy them in `Scripts/package_app.sh`.

### 5. `Scripts/package_app.sh`

After the `cp "Resources/AppIcon.icns" ...` line (line 47), add:

```bash
cp Resources/MenuBarIcon.png \
   Resources/MenuBarIcon@2x.png \
   Resources/MenuBarIcon@3x.png \
   "${CONTENTS}/Resources/"
```

Explicit filenames (not a glob) so that if any of the three PNGs goes missing, `cp` fails loudly with the exact filename instead of passing an unexpanded `MenuBarIcon*.png` literal to `cp`. Must happen before `codesign` so the signature covers the new resources.

## Validation

- `swift build -c release` succeeds. `.build/release/Shun_Shun.bundle` is gone and `.build/.../Shun.build/DerivedSources/resource_bundle_accessor.swift` is no longer generated for the Shun target.
- `./Scripts/package_app.sh` produces `Shun.app/Contents/Resources/` containing `AppIcon.icns` + 3 `MenuBarIcon*.png`.
- `cp -r Shun.app /Applications/`, then **`rm -rf .build`** (or switch to a different user account), then `open /Applications/Shun.app`. Menu bar icon renders and adapts to Light/Dark. The `rm -rf .build` step is load-bearing: without it the dev machine's build path masks any accessor-related regression. See "Why this wasn't caught in dev" above.
- `codesign --verify --strict --verbose=2 Shun.app` passes.

## Release / rollout

This is a launch-time crash on affected machines, which means **Sparkle cannot deliver the fix** — a user whose app crashes on `open` never gets far enough for the updater to run. The fix ships via auto-update only for users who currently launch successfully.

Before tagging the release:

- `git log -- Package.swift` to determine when `resources: [.process("Resources")]` was introduced, and which shipped versions are affected. If all shipped 1.x versions contain it, the reason most users haven't reported the crash is environmental (macOS version, cached bundles, prior installs). Assume the blast radius is broad.
- Release notes should call out the crash and link a **manual download** (DMG/zip) for users stuck on the broken version.
- Consider pinning a notice on the project page / README pointing at the manual download URL.

## Non-goals / YAGNI

- **SF Symbol fallback for `swift run`**: dropped. `swift run` is not a primary debug path for this app (no `SUFeedURL`, no embedded framework). If `NSImage(named:)` returns an empty image, the status item button is still clickable.
- **Keeping `Bundle.module` working elsewhere**: there are no other consumers. If one is added later, decide then.

## Rollback

Single-commit revert restores `resources: [.process("Resources")]`, the `Sources/Shun/Resources/` directory, `Bundle.module` usage, and removes the `cp` line from `package_app.sh`.

## Follow-up — 2026-04-22: dependency resource bundles

The original fix above was target-scoped: it removed `Bundle.module` from the *Shun* target. It did not address SwiftPM dependencies that ship their own resources, which hit the same `Bundle.module` accessor via the same mechanism.

### What crashed

1.0.4 crashed on the dev Mac the moment a `KeyboardShortcuts.Recorder` SwiftUI view was laid out:

```
_assertionFailure
closure #1 in variable initialization of NSBundle.module    ← SPM-generated
KeyboardShortcuts.RecorderCocoa.init(for:onChange:)
```

`KeyboardShortcuts` 1.15.0 declares resources (localized strings for the Recorder's accessibility labels), so SwiftPM emits `KeyboardShortcuts_KeyboardShortcuts.bundle` into `.build/release/` and generates a `Bundle.module` accessor inside the dependency. `Scripts/package_app.sh` only copied Sparkle + hand-picked loose files, so the `.app` shipped without that bundle — same failure mode as the original 1.0.3 / 1.0.4 menu-bar-icon crash, one layer deeper in the dependency graph.

Why it reproduced on the *build* machine this time: `rm -rf .build` (or any SwiftPM rebuild that relocated artifacts) removed the hardcoded dev-machine fallback path that was silently keeping the shipped binary alive. Exactly the scenario the original "Why this wasn't caught in dev" section called out as the load-bearing test case.

### Fix

Generalize `Scripts/package_app.sh` to auto-embed every SPM-generated resource bundle from `.build/release/` into `Contents/Resources/`, with a self-exclusion so the Shun target's (non-existent) bundle can't sneak back in without a loud failure:

```bash
shopt -s nullglob
for bundle in "${BUILD_DIR}"/*_*.bundle; do
    name=$(basename "${bundle}")
    [[ "${name}" == "${APP_NAME}_${APP_NAME}.bundle" ]] && continue
    cp -R "${bundle}" "${CONTENTS}/Resources/"
done
shopt -u nullglob
```

The final `codesign "${APP_BUNDLE}"` at the end of the script seals these as ordinary resources — no separate signing step needed since they contain no Mach-O.

### Guardrail

CLAUDE.md gets a second bullet under Critical Conventions: "SPM dependency resource bundles must be embedded." The rule references this doc and spells out the glob-vs-explicit-list trade-off (glob preferred so new resource-bearing deps are auto-picked-up).

### Validation note

`Validation` in the original section is now insufficient on its own. A release smoke test must `rm -rf .build` (or use a fresh user account) before opening `/Applications/Shun.app` *and* exercise each SPM dependency's resource path — for `KeyboardShortcuts` that means opening Settings and scrolling to a `Recorder` row. Otherwise the `.build` fallback continues to mask the bug class indefinitely. Folding this into the `verify-release` agent is a worthwhile next step.

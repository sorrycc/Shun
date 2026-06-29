#!/bin/bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="Shun"
BUNDLE_ID="com.shun.Shun"
VERSION="${VERSION:-$(cat VERSION)}"
SHORT_VERSION="${VERSION}"
BUILD_DIR=".build/release"
APP_BUNDLE="${APP_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"
FRAMEWORKS="${CONTENTS}/Frameworks"
SIGN_IDENTITY="Developer ID Application: chen cheng (KU8S35TEW8)"

# ---- Sparkle config: single source of truth for public key + feed URL ----
SPARKLE_CONFIG="Scripts/sparkle.config"
if [[ ! -f "${SPARKLE_CONFIG}" ]]; then
    echo "FAIL: ${SPARKLE_CONFIG} not found." >&2
    exit 1
fi
# shellcheck disable=SC1090
source "${SPARKLE_CONFIG}"

if [[ -z "${SPARKLE_PUBLIC_KEY:-}" || -z "${SPARKLE_FEED_URL:-}" ]]; then
    echo "FAIL: ${SPARKLE_CONFIG} is missing SPARKLE_PUBLIC_KEY or SPARKLE_FEED_URL." >&2
    echo "      See docs/designs/2026-04-17-r2-upload-and-auto-update.md for one-time setup." >&2
    exit 1
fi

# ---- Telemetry config (optional): endpoint URL + HMAC key ----
# If Scripts/telemetry.config exists, its values get baked into Info.plist so
# CrashReporter / TelemetryService can activate. Absent config → services stay
# silent (matches the SUFeedURL gate pattern).
TELEMETRY_CONFIG="Scripts/telemetry.config"
if [[ -f "${TELEMETRY_CONFIG}" ]]; then
    # shellcheck disable=SC1090
    source "${TELEMETRY_CONFIG}"
fi

if [[ -n "${TELEMETRY_ENDPOINT_URL:-}" && -n "${TELEMETRY_INGEST_KEY:-}" ]]; then
    TELEMETRY_PLIST_KEYS="    <key>TelemetryEndpointURL</key>
    <string>${TELEMETRY_ENDPOINT_URL}</string>
    <key>TelemetryIngestKey</key>
    <string>${TELEMETRY_INGEST_KEY}</string>"
    echo "Telemetry endpoint: ${TELEMETRY_ENDPOINT_URL}"
else
    TELEMETRY_PLIST_KEYS=""
    echo "Telemetry: disabled (no Scripts/telemetry.config or env vars)"
fi

# Validate Ed25519 public key shape: 32 bytes -> 44 chars base64 ending with '='.
if ! [[ "${SPARKLE_PUBLIC_KEY}" =~ ^[A-Za-z0-9+/]{43}=$ ]]; then
    echo "FAIL: SPARKLE_PUBLIC_KEY in ${SPARKLE_CONFIG} doesn't look like a valid Ed25519 base64 key." >&2
    echo "      Expected 44 chars ending with '='. Got: '${SPARKLE_PUBLIC_KEY}'" >&2
    exit 1
fi

# ---- Patch KeyboardShortcuts' generated resource bundle accessor (GH #1) ----
# SwiftPM's generated accessor for the KeyboardShortcuts dep looks up the bundle at
#   Bundle.main.bundleURL.appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle")
# For a packaged .app that's the .app root — placing files there breaks codesign
# ("unsealed contents present in the bundle root"). We redirect the lookup to
# Bundle.main.resourceURL (= Contents/Resources/) and embed the bundle there.
#
# Three-pass build: pass 1 fresh-builds so the accessor file exists; pass 2 is a
# no-op that puts SPM's build DB into a state where DerivedSources are considered
# up-to-date; we then patch + chmod 444 so pass 3 skips regeneration and compiles
# our patched version. A single-pass "patch after pass 1 → pass 2 with chmod 444"
# fails with "invalid access" because SPM still tries to re-emit the file from
# its template on the second build after a fresh one.
ACCESSOR=".build/arm64-apple-macosx/release/KeyboardShortcuts.build/DerivedSources/resource_bundle_accessor.swift"

echo "Building ${APP_NAME} ${VERSION} (release, pass 1/3)..."
swift build -c release

echo "Building ${APP_NAME} ${VERSION} (release, pass 2/3 — settle SPM incremental state)..."
swift build -c release

if [[ ! -f "${ACCESSOR}" ]]; then
    echo "FAIL: ${ACCESSOR} not found after pass 2." >&2
    exit 1
fi

# Idempotent: grep skips the patch if we've already applied it and chmod 644
# hasn't been restored by a dev-run of `swift build`.
if ! grep -q "Bundle\.main\.resourceURL" "${ACCESSOR}"; then
    echo "Patching KeyboardShortcuts resource_bundle_accessor.swift..."
    chmod 644 "${ACCESSOR}"
    sed -i.bak 's|Bundle\.main\.bundleURL\.appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts\.bundle")\.path|(Bundle.main.resourceURL ?? Bundle.main.bundleURL).appendingPathComponent("KeyboardShortcuts_KeyboardShortcuts.bundle").path|' "${ACCESSOR}"
    rm "${ACCESSOR}.bak"
fi
chmod 444 "${ACCESSOR}"

echo "Building ${APP_NAME} ${VERSION} (release, pass 3/3 — with patched accessor)..."
swift build -c release

# Restore writable so future `swift build` invocations (dev workflow, `swift run`)
# can regenerate the accessor from SPM's template without "invalid access" errors.
# The next package_app.sh run re-patches if needed.
chmod 644 "${ACCESSOR}"

echo "Creating app bundle..."
rm -rf "${APP_BUNDLE}"
mkdir -p "${CONTENTS}/MacOS"
mkdir -p "${CONTENTS}/Resources"
mkdir -p "${FRAMEWORKS}"

cp "${BUILD_DIR}/${APP_NAME}" "${CONTENTS}/MacOS/${APP_NAME}"
cp "Resources/AppIcon.icns" "${CONTENTS}/Resources/AppIcon.icns"
# Menu bar icons: loaded via NSImage(named:) which auto-picks @2x/@3x from Contents/Resources/.
# Explicit filenames (not a glob) so a missing file fails cp loudly with the exact name.
cp "Resources/MenuBarIcon.png" \
   "Resources/MenuBarIcon@2x.png" \
   "Resources/MenuBarIcon@3x.png" \
   "${CONTENTS}/Resources/"

# swift build produces a binary without @executable_path/../Frameworks in LC_RPATH.
# Xcode adds this automatically; SPM + hand-rolled bundling does not. Without it,
# dyld looks for Sparkle.framework at Contents/MacOS/Sparkle.framework (wrong path)
# and the app crashes at launch with "Library not loaded". Patch the Mach-O before
# signing (modifying rpath invalidates any prior signature).
echo "Patching rpath so dyld finds Contents/Frameworks/..."
install_name_tool -add_rpath "@executable_path/../Frameworks" \
                  "${CONTENTS}/MacOS/${APP_NAME}"

# ---- Embed Sparkle.framework ----
# Sparkle ships as an XCFramework via SPM binaryTarget. Find and copy the macOS slice.
SPARKLE_XCFRAMEWORK=$(find .build/artifacts -type d -name "Sparkle.xcframework" | head -n 1)
if [[ -z "${SPARKLE_XCFRAMEWORK}" ]]; then
    echo "FAIL: Sparkle.xcframework not found under .build/artifacts." >&2
    echo "      Did 'swift build -c release' complete successfully?" >&2
    exit 1
fi
SPARKLE_FRAMEWORK=$(find "${SPARKLE_XCFRAMEWORK}" -type d -name "Sparkle.framework" | head -n 1)
if [[ -z "${SPARKLE_FRAMEWORK}" ]]; then
    echo "FAIL: Sparkle.framework not found inside ${SPARKLE_XCFRAMEWORK}." >&2
    exit 1
fi
echo "Embedding Sparkle from ${SPARKLE_FRAMEWORK}..."
cp -R "${SPARKLE_FRAMEWORK}" "${FRAMEWORKS}/"

# ---- Auto-copy SwiftPM resource bundles ----
# Any SwiftPM target declaring `resources:` emits <Pkg>_<Target>.bundle next to
# the release binary. At runtime Bundle.module (via the patched accessor
# above) looks for it at Contents/Resources/. Shipping without = Bundle.module
# fatalError on first use — the 1.0.4 ship crash. Auto-copy means any new
# package with resources Just Works with no future package_app.sh edits.
BIN_PATH=$(swift build --show-bin-path -c release)
while IFS= read -r -d '' bundle_path; do
    bundle_name=$(basename "${bundle_path}")
    echo "[COPY] ${bundle_name}"
    cp -R "${bundle_path}" "${CONTENTS}/Resources/"
done < <(find "${BIN_PATH}" -maxdepth 1 -type d -name '*.bundle' -print0 2>/dev/null)

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
    <string>14.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
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
${TELEMETRY_PLIST_KEYS}
</dict>
</plist>
PLIST

# ---- Sign any auto-copied SwiftPM resource bundles ----
# Nested bundles must be signed before the outer `codesign --force` seals them,
# otherwise notarization rejects any Mach-O helpers inside. `--options runtime`
# only applies to bundles with Mach-O content; fall back without it for
# resource-only bundles (most SwiftPM resource bundles — KeyboardShortcuts'
# bundle is asset-only, for example).
while IFS= read -r -d '' spm_bundle; do
    bundle_name=$(basename "${spm_bundle}")
    echo "Code signing ${bundle_name}..."
    codesign --force --options runtime --timestamp \
             --sign "${SIGN_IDENTITY}" \
             "${spm_bundle}" 2>/dev/null || \
        codesign --force --timestamp \
                 --sign "${SIGN_IDENTITY}" \
                 "${spm_bundle}"
done < <(find "${CONTENTS}/Resources" -maxdepth 1 -type d -name '*.bundle' -print0 2>/dev/null)

# ---- Pre-sign structural verification ----
./Scripts/verify_app.sh "${APP_BUNDLE}" --pre-sign

# ---- Code sign: inside-out, runtime hardened ----
# Sparkle.framework contains nested XPC services and helpers that must be signed first.
# We re-sign them with our Developer ID so notarization accepts the whole bundle under our team.
EMBEDDED_SPARKLE="${FRAMEWORKS}/Sparkle.framework"
echo "Code signing Sparkle helpers (inside-out)..."

# Find deepest bundles first (xpc, app), then the framework itself.
# Use null delimiters so paths with spaces survive.
while IFS= read -r -d '' bundle; do
    codesign --force --options runtime --timestamp \
             --sign "${SIGN_IDENTITY}" \
             "${bundle}"
done < <(find "${EMBEDDED_SPARKLE}" \
            \( -name "*.xpc" -o -name "*.app" \) \
            -type d -print0 \
            | sort -rz)

# Also sign any raw helper executables inside Versions/*/ (e.g., Autoupdate)
for helper in "${EMBEDDED_SPARKLE}/Versions/"*/Autoupdate; do
    [[ -f "${helper}" ]] || continue
    codesign --force --options runtime --timestamp \
             --sign "${SIGN_IDENTITY}" \
             "${helper}"
done

echo "Code signing Sparkle.framework..."
codesign --force --options runtime --timestamp \
         --sign "${SIGN_IDENTITY}" \
         "${EMBEDDED_SPARKLE}"

echo "Code signing main app with Developer ID + hardened runtime..."
codesign --force --options runtime --timestamp \
         --sign "${SIGN_IDENTITY}" \
         "${APP_BUNDLE}"

echo "Verifying signature..."
codesign --verify --strict --verbose=2 "${APP_BUNDLE}"

# ---- Post-sign structural + codesign verification ----
# Redundant with the line above but catches regressions in verify_app.sh's
# codesign check — defense in depth for the exact class (silent shipping) this
# verifier exists to prevent.
./Scripts/verify_app.sh "${APP_BUNDLE}"

echo "Done! ${APP_BUNDLE} is signed (not yet notarized)."
echo "  - To install locally: cp -r ${APP_BUNDLE} /Applications/"
echo "  - To cut a release:   ./Scripts/release.sh"

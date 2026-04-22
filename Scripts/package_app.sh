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

# Validate Ed25519 public key shape: 32 bytes -> 44 chars base64 ending with '='.
if ! [[ "${SPARKLE_PUBLIC_KEY}" =~ ^[A-Za-z0-9+/]{43}=$ ]]; then
    echo "FAIL: SPARKLE_PUBLIC_KEY in ${SPARKLE_CONFIG} doesn't look like a valid Ed25519 base64 key." >&2
    echo "      Expected 44 chars ending with '='. Got: '${SPARKLE_PUBLIC_KEY}'" >&2
    exit 1
fi

echo "Building ${APP_NAME} ${VERSION} (release)..."
swift build -c release

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

# ---- Embed SPM-generated dependency resource bundles ----
# Third-party SPM targets that declare resources (e.g. KeyboardShortcuts's
# localized strings) get an auto-generated Bundle.module accessor that probes
# Bundle.main.resourceURL first, then falls back to a hardcoded .build path.
# The Shun target has no resources: declaration (see docs/designs/2026-04-20-spm-resource-bundle-fix.md),
# but dependencies can, and the fallback only exists on the build machine, so
# any .app shipped without these bundles fatalErrors on first use (1.0.4 crash).
shopt -s nullglob
for bundle in "${BUILD_DIR}"/*_*.bundle; do
    name=$(basename "${bundle}")
    # Defensive: the Shun target intentionally produces no bundle; skip if one
    # sneaks back in so we don't paper over a re-introduced resources: decl.
    [[ "${name}" == "${APP_NAME}_${APP_NAME}.bundle" ]] && continue
    echo "Embedding SPM resource bundle: ${name}"
    cp -R "${bundle}" "${CONTENTS}/Resources/"
done
shopt -u nullglob

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
</dict>
</plist>
PLIST

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

echo "Done! ${APP_BUNDLE} is signed (not yet notarized)."
echo "  - To install locally: cp -r ${APP_BUNDLE} /Applications/"
echo "  - To cut a release:   ./Scripts/release.sh"

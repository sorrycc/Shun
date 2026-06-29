#!/bin/bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

APP_NAME="Shun"
APP_BUNDLE="${APP_NAME}.app"
SIGN_IDENTITY="Developer ID Application: chen cheng (KU8S35TEW8)"
TEAM_ID="KU8S35TEW8"
NOTARY_PROFILE="shun-notary"

# ---- preflight: fail fast on config problems BEFORE slow work ----
preflight() {
    echo "Preflight checks..."

    if ! security find-identity -v -p codesigning | grep -q "${TEAM_ID}"; then
        echo "FAIL: Developer ID cert for team ${TEAM_ID} not in keychain." >&2
        echo "      Install the Developer ID Application certificate from developer.apple.com." >&2
        exit 1
    fi

    if ! nt_out=$(xcrun notarytool history --keychain-profile "${NOTARY_PROFILE}" 2>&1); then
        if echo "${nt_out}" | grep -qiE "keychain|profile|could not find|no.*item"; then
            echo "FAIL: '${NOTARY_PROFILE}' keychain profile missing." >&2
            echo "      Set it up once with:" >&2
            echo "        xcrun notarytool store-credentials ${NOTARY_PROFILE} \\" >&2
            echo "            --apple-id <your-apple-id> \\" >&2
            echo "            --team-id ${TEAM_ID} \\" >&2
            echo "            --password <app-specific-password>" >&2
        else
            echo "FAIL: notarytool rejected '${NOTARY_PROFILE}' (profile exists, but the request failed):" >&2
            echo "${nt_out}" | sed 's/^/      /' >&2
            echo "      A 403 'agreement' error means an Apple Developer agreement needs accepting" >&2
            echo "      at https://developer.apple.com/account/ (and App Store Connect agreements)." >&2
        fi
        exit 1
    fi

    echo "  ok: Developer ID cert found"
    echo "  ok: '${NOTARY_PROFILE}' notary profile found"
}
preflight

# ---- build + sign the app ----
./Scripts/package_app.sh

# single source of truth for version — read from the bundle we just built
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
# --timeout 20m caps wall-clock if Apple's notary service hiccups (default 3h).
# tee captures the submission ID for later `notarytool log <id>` lookups.
echo "Submitting to Apple notary service (up to 20 min)..."
xcrun notarytool submit "${DMG_PATH}" \
    --keychain-profile "${NOTARY_PROFILE}" --wait --timeout 20m \
    | tee "${NOTARY_LOG}"

# ---- staple both artifacts ----
# Single-submission-DMG usually publishes tickets for the app inside too,
# but it isn't explicitly guaranteed by Apple docs. Fall back to a separate
# app submission if stapling the app fails with "no ticket available".
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

# ---- launch smoke test on the stapled app ----
# Catches dyld/Bundle.module crashes that would abort the app at main()
# before any UI renders — the exact crash class seen in 1.0.2 / 1.0.4.
echo "Launch smoke test on stapled app..."
./Scripts/verify_app.sh "${APP_BUNDLE}" --smoke

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

    # Capture output into a variable first. Piping codesign into `grep -q`
    # trips `set -o pipefail`: grep -q exits early on match, codesign gets
    # SIGPIPE, pipeline exits non-zero even though the signature is fine.
    local codesign_out
    codesign_out=$(codesign -d -v "$bundle" 2>&1)

    # Hardened runtime is a property of executables only — DMGs are disk
    # images and always report flags=0x0(none), so skip this check for DMGs.
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

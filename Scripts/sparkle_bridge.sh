#!/usr/bin/env bash
# Publishes the Go version of Shun to the Sparkle appcast on R2, once, so
# that installs of the Swift versions (1.0.10 and earlier), which update
# with Sparkle, update to it. From then on the app updates itself with
# MyGo's updater, from the GitHub releases.
#
# Usage: Scripts/sparkle_bridge.sh [--dry-run]
#
# Run it after the Release workflow published the version of mygo.json. It
# builds the same version locally, signed with the Developer ID, notarized
# (the shun-notary profile) and with the telemetry of
# Scripts/telemetry.config, all set in mygo.json for the build only, packs the app as Sparkle expects
# (build/Shun-<version>.zip, .dmg and .sha256, and ./Shun.app), and runs
# Scripts/upload_r2.ts, which signs the zip with the Sparkle key and adds it
# to appcast.xml. It needs .env (R2) and Sparkle's sign_update.
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="$(jq -r .version mygo.json)"
IDENTITY="Developer ID Application: chen cheng (KU8S35TEW8)"
if [ -n "$(git status --porcelain mygo.json)" ]; then
    echo "mygo.json has changes. Commit them first." >&2
    exit 1
fi
# shellcheck source=/dev/null
. Scripts/telemetry.config
: "${TELEMETRY_ENDPOINT_URL:?missing in Scripts/telemetry.config}" "${TELEMETRY_INGEST_KEY:?missing in Scripts/telemetry.config}"

echo "==> Building Shun $VERSION, signed and notarized"
trap 'git checkout -- mygo.json' EXIT
jq --arg identity "$IDENTITY" --arg url "$TELEMETRY_ENDPOINT_URL" --arg key "$TELEMETRY_INGEST_KEY" \
    '.macos.signingIdentity = $identity | .macos.notarize = {keychainProfile: "shun-notary"}
     | .macos.infoPlist += {TelemetryEndpointURL: $url, TelemetryIngestKey: $key}' \
    mygo.json > mygo.json.tmp
mv mygo.json.tmp mygo.json
go tool mygo build -platform darwin/arm64
git checkout -- mygo.json
trap - EXIT
APP="build/darwin-arm64/Shun.app"
DMG="build/darwin-arm64/Shun $VERSION arm64.dmg"
xcrun stapler staple "$APP" >/dev/null || true
xcrun stapler validate "$APP"
spctl -a -t exec -vv "$APP"

echo "==> Packing for Sparkle"
rm -rf Shun.app "build/Shun-$VERSION.zip"
ditto "$APP" Shun.app
ditto -c -k --sequesterRsrc --keepParent "$APP" "build/Shun-$VERSION.zip"
cp "$DMG" "build/Shun-$VERSION.dmg"
(cd build && shasum -a 256 "Shun-$VERSION.dmg" "Shun-$VERSION.zip" > "Shun-$VERSION.sha256")

echo "==> Publishing to the appcast"
bun Scripts/upload_r2.ts "$@"

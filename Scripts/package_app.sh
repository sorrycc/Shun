#!/bin/bash
set -euo pipefail

APP_NAME="HotApp"
BUNDLE_ID="com.hotapp.HotApp"
VERSION="1.0.0"
SHORT_VERSION="1.0"
BUILD_DIR=".build/release"
APP_BUNDLE="${APP_NAME}.app"
CONTENTS="${APP_BUNDLE}/Contents"

echo "Building ${APP_NAME} (release)..."
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
    <string>14.0</string>
    <key>LSUIElement</key>
    <true/>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

echo "Code signing (ad-hoc)..."
codesign --force --sign - "${APP_BUNDLE}"

echo "Done! ${APP_BUNDLE} is ready."
echo "To install: cp -r ${APP_BUNDLE} /Applications/"

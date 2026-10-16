#!/usr/bin/env bash
# Runs Shun Dev from source with live reload: it builds a bare binary, runs
# it, and builds and restarts it when a Go file, an asset or mygo.config.ts
# changes. A build that fails keeps the running one. Quit Shun Dev, or press
# Ctrl-C, to stop.
#
# A bare binary rather than mygo dev's "Shun Dev" bundle: macOS 26 keeps the
# menu bar icon of that bundle hidden, even when System Settings allows it.
# Run from a terminal, it also has the terminal's Accessibility permission,
# so its event tap takes the keys.
#
# SHUN_DATA_DIR gives it its own shortcuts, settings and usage, seeded from
# the installed Shun's the first time. The installed Shun takes the same
# keys: when it runs, this offers to quit it for the session and opens it
# again when Shun Dev stops.
#
# Usage: Scripts/dev.sh [--reseed]
#   --reseed   copy the installed Shun's data again, replacing Shun Dev's
# SHUN_DATA_DIR picks another data folder (default: "Shun Dev" beside Shun's).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SUPPORT="$HOME/Library/Application Support"
REAL="$SUPPORT/Shun"
export SHUN_DATA_DIR="${SHUN_DATA_DIR:-$SUPPORT/Shun Dev}"
OUT="$ROOT/.mygo/dev"
BIN="$OUT/shun"
STAMP="$OUT/built"

if [ "${1:-}" = "--reseed" ]; then
    rm -f "$SHUN_DATA_DIR/shortcuts.json" "$SHUN_DATA_DIR/usage.json" "$SHUN_DATA_DIR/settings.json"
elif [ $# -gt 0 ]; then
    sed -n '2,19s/^# \{0,1\}//p' "$0" >&2
    exit 1
fi

# Seed from the installed Shun once. Its keys may still be in the Swift
# app's UserDefaults: Shun Dev migrates them from there, as a release does.
mkdir -p "$SHUN_DATA_DIR" "$OUT"
if [ ! -f "$SHUN_DATA_DIR/shortcuts.json" ] && [ -f "$REAL/shortcuts.json" ]; then
    for f in shortcuts.json usage.json settings.json; do
        if [ -f "$REAL/$f" ]; then
            cp "$REAL/$f" "$SHUN_DATA_DIR/$f"
        fi
    done
    echo "==> seeded $SHUN_DATA_DIR from $REAL"
fi

# The installed Shun, not this one: its bundle ID is com.shun.Shun exactly.
REOPEN=
if [ -n "$(lsappinfo find bundleid=com.shun.Shun 2>/dev/null)" ]; then
    echo "==> The installed Shun is running: its event tap takes the shortcuts' keys too."
    if [ -t 0 ]; then
        read -r -p "Quit it until Shun Dev stops? [y/N] " answer || echo
        if [[ "$answer" == [yY]* ]]; then
            osascript -e 'quit app id "com.shun.Shun"'
            REOPEN=1
        fi
    fi
fi

cd "$ROOT"
APP=""
stop() {
    if [ -n "$APP" ] && kill -0 "$APP" 2>/dev/null; then
        kill "$APP" 2>/dev/null || true
        wait "$APP" 2>/dev/null || true
    fi
    APP=""
}
finish() {
    stop
    if [ -n "$REOPEN" ]; then
        echo "==> opening Shun again"
        open -b com.shun.Shun
    fi
}
trap finish EXIT
trap 'exit 0' INT TERM

build() {
    touch "$STAMP.next"
    if go build -o "$BIN.next" ./cmd/shun; then
        mv "$BIN.next" "$BIN"
        mv "$STAMP.next" "$STAMP"
        return 0
    fi
    rm -f "$STAMP.next"
    return 1
}

# changed reports whether a source file is newer than the last build.
changed() {
    [ -n "$(find cmd internal resources mygo.config.ts go.mod go.sum -newer "$STAMP" -type f \
        \( -name '*.go' -o -name '*.png' -o -name '*.ts' -o -name 'go.mod' -o -name 'go.sum' \) \
        -not -name '*_test.go' -print -quit 2>/dev/null)" ]
}

start() {
    "$BIN" &
    APP=$!
    echo "==> Shun Dev is running (pid $APP, data in $SHUN_DATA_DIR)"
}

echo "==> building"
build
start
while true; do
    sleep 1
    if ! kill -0 "$APP" 2>/dev/null; then
        echo "==> Shun Dev quit"
        exit 0
    fi
    if changed; then
        echo "==> rebuilding"
        if build; then
            stop
            start
        else
            echo "==> the build failed; Shun Dev keeps running the last one"
        fi
    fi
done

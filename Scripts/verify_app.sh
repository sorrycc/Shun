#!/bin/bash
# verify_app.sh — structural + signing checks on a packaged .app bundle.
#
# Exists because Shun (sibling app, same dev team) shipped two launch-time
# crashes that would have been caught here: missing Sparkle.framework at
# @rpath, and missing SwiftPM resource bundle tripping Bundle.module's
# fatalError. See docs/designs/2026-04-24-package-verifier.md.
#
# Usage: verify_app.sh <path-to-App.app> [--smoke]

set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

if [[ $# -lt 1 || "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    cat <<EOF
usage: verify_app.sh <path-to-App.app> [--smoke]

Checks: bundle sanity, embedded frameworks, @rpath resolution, SwiftPM
resource bundle completeness, codesign verification, optional launch probe.

Exit 0 = all green. Exit 1 = any [FAIL]. [WARN] never fails.
EOF
    exit 0
fi

APP="${1%/}"
SMOKE=0
PRE_SIGN=0
shift || true
while [[ $# -gt 0 ]]; do
    case "$1" in
        --smoke)    SMOKE=1 ;;
        --pre-sign) PRE_SIGN=1 ;;
        *) echo "unknown arg: $1" >&2; exit 1 ;;
    esac
    shift
done

FAIL_COUNT=0
fail() { echo "[FAIL] $*" >&2; FAIL_COUNT=$((FAIL_COUNT + 1)); }
ok()   { echo "[OK]   $*"; }
warn() { echo "[WARN] $*"; }

# -- Check 1: bundle sanity --------------------------------------------------
if [[ ! -d "${APP}" ]]; then
    echo "[FAIL] not a directory: ${APP}" >&2
    exit 1
fi
INFO_PLIST="${APP}/Contents/Info.plist"
if [[ ! -f "${INFO_PLIST}" ]]; then
    fail "Info.plist missing at ${INFO_PLIST}"
    exit 1
fi
if ! APP_NAME=$(plutil -extract CFBundleExecutable raw -o - "${INFO_PLIST}" 2>/dev/null); then
    fail "Info.plist has no CFBundleExecutable"
    exit 1
fi
BINARY="${APP}/Contents/MacOS/${APP_NAME}"
if [[ ! -f "${BINARY}" ]]; then
    fail "binary missing: ${BINARY}"
    exit 1
fi
if ! file -b "${BINARY}" | grep -q "Mach-O"; then
    fail "${BINARY} is not Mach-O ($(file -b "${BINARY}"))"
    exit 1
fi
ok "bundle layout: ${APP}"

# -- Check 2: embedded Sparkle.framework -------------------------------------
SPARKLE_BIN="${APP}/Contents/Frameworks/Sparkle.framework/Sparkle"
if [[ ! -f "${SPARKLE_BIN}" ]]; then
    fail "Sparkle.framework missing at Contents/Frameworks/"
else
    if ! file -b "${SPARKLE_BIN}" | grep -q "Mach-O"; then
        fail "Sparkle binary not Mach-O: $(file -b "${SPARKLE_BIN}")"
    else
        ok "Sparkle.framework embedded"
    fi
fi

# -- Check 3: LC_RPATH includes @executable_path/../Frameworks ---------------
if otool -l "${BINARY}" 2>/dev/null \
   | awk '/cmd LC_RPATH/{flag=1; next} flag && /path /{print; flag=0}' \
   | grep -q "@executable_path/../Frameworks"; then
    ok "LC_RPATH @executable_path/../Frameworks present"
else
    fail "LC_RPATH @executable_path/../Frameworks not set on ${BINARY}"
fi

# -- Check 4: every @rpath/... dep resolves inside Contents/Frameworks/ ------
RPATH_DEPS=$(otool -L "${BINARY}" 2>/dev/null \
             | awk '$1 ~ /^@rpath\//{print $1}')
resolved=0
unresolved=0
while IFS= read -r dep; do
    [[ -z "${dep}" ]] && continue
    rel="${dep#@rpath/}"
    if [[ -f "${APP}/Contents/Frameworks/${rel}" ]]; then
        resolved=$((resolved + 1))
    else
        fail "@rpath dep unresolved: ${dep} -> Contents/Frameworks/${rel}"
        unresolved=$((unresolved + 1))
    fi
done <<< "${RPATH_DEPS}"
if [[ ${unresolved} -eq 0 ]]; then
    ok "@rpath deps resolve (${resolved} checked)"
fi

# -- Check 5: SwiftPM resource bundles shipped -------------------------------
BIN_PATH=""
if BIN_PATH=$(swift build --show-bin-path -c release 2>/dev/null); then :; fi

build_bundles=()
if [[ -n "${BIN_PATH}" && -d "${BIN_PATH}" ]]; then
    while IFS= read -r -d '' b; do
        build_bundles+=("$b")
    done < <(find "${BIN_PATH}" -maxdepth 1 -type d -name '*.bundle' -print0 2>/dev/null)
fi

if [[ ${#build_bundles[@]} -eq 0 ]]; then
    warn "no SwiftPM bundles in build output (post-hoc verification mode)"
else
    missing=0
    for bundle_path in "${build_bundles[@]}"; do
        name=$(basename "${bundle_path}")
        if [[ ! -d "${APP}/Contents/Resources/${name}" ]]; then
            fail "${name} built by SwiftPM but missing from Contents/Resources/"
            missing=$((missing + 1))
        fi
    done
    if [[ ${missing} -eq 0 ]]; then
        ok "SwiftPM resource bundles shipped (${#build_bundles[@]} checked)"
    fi

    # Orphan bundles in Resources/ that don't match build output → warn
    shipped_bundles=()
    while IFS= read -r -d '' b; do
        shipped_bundles+=("$b")
    done < <(find "${APP}/Contents/Resources" -maxdepth 1 -type d -name '*.bundle' -print0 2>/dev/null)
    for shipped in "${shipped_bundles[@]:-}"; do
        [[ -z "${shipped}" ]] && continue
        name=$(basename "${shipped}")
        if [[ ! -d "${BIN_PATH}/${name}" ]]; then
            warn "shipped bundle has no matching build output: ${name} (stale?)"
        fi
    done
fi

# -- Check 6: codesign --verify (skipped with --pre-sign or for unsigned) ----
if [[ ${PRE_SIGN} -eq 1 ]]; then
    warn "--pre-sign: skipping codesign --verify (bundle not yet re-signed)"
elif codesign -dv "${APP}" >/dev/null 2>&1; then
    if codesign --verify --deep --strict --verbose=2 "${APP}" >/dev/null 2>&1; then
        ok "codesign --verify --deep --strict passed"
    else
        fail "codesign --verify --deep --strict failed for ${APP}"
        codesign --verify --deep --strict --verbose=2 "${APP}" 2>&1 | sed 's/^/       /' >&2 || true
    fi
else
    warn "${APP} is unsigned — skipping codesign --verify"
fi

# -- Check 7: launch smoke test (opt-in) -------------------------------------
if [[ ${SMOKE} -eq 1 ]]; then
    SMOKE_PID=""
    cleanup() {
        if [[ -n "${SMOKE_PID}" ]] && kill -0 "${SMOKE_PID}" 2>/dev/null; then
            kill "${SMOKE_PID}" 2>/dev/null || true
        fi
    }
    trap cleanup EXIT

    # Launch a fresh instance without activating it.
    if ! open -gn "${APP}" 2>/dev/null; then
        fail "open -gn failed to launch ${APP}"
    else
        # Capture the newest matching process — `pgrep -n` = newest PID.
        for _ in 1 2 3 4 5 6 7 8 9 10; do
            SMOKE_PID=$(pgrep -n -f "${BINARY}" 2>/dev/null | head -1 || true)
            [[ -n "${SMOKE_PID}" ]] && break
            sleep 0.5
        done

        if [[ -z "${SMOKE_PID}" ]]; then
            fail "smoke: ${APP_NAME} process never appeared"
        else
            # Wait the remainder of the 5s window and re-check.
            sleep 3
            if kill -0 "${SMOKE_PID}" 2>/dev/null; then
                ok "smoke: ${APP_NAME} pid ${SMOKE_PID} alive after 5s"
            else
                fail "smoke: ${APP_NAME} pid ${SMOKE_PID} exited early (dyld/Bundle.module crash?)"
            fi
        fi
    fi
fi

if [[ ${FAIL_COUNT} -gt 0 ]]; then
    echo "" >&2
    echo "verify_app.sh: ${FAIL_COUNT} failure(s)" >&2
    exit 1
fi
echo "verify_app.sh: all checks passed"

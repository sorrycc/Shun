#!/bin/bash
# Copy gitignored local files (R2 creds, telemetry config) from another Shun clone
# so this checkout can build & release without re-running one-time setup.
#
# Usage: Scripts/sync_local_secrets.sh <path-to-source-shun-clone>

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <path-to-source-shun-clone>" >&2
  exit 2
fi

SRC="${1%/}"
DST="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! -d "$SRC" ]]; then
  echo "error: source path does not exist: $SRC" >&2
  exit 1
fi
if [[ ! -f "$SRC/mygo.config.ts" && ! -f "$SRC/Package.swift" ]]; then
  echo "error: $SRC does not look like a Shun clone (no mygo.config.ts or Package.swift)" >&2
  exit 1
fi
if [[ "$SRC" -ef "$DST" ]]; then
  echo "error: source and destination are the same directory" >&2
  exit 1
fi

FILES=(
  ".env"
  "Scripts/telemetry.config"
)

copied=0
skipped=0
missing=0

for rel in "${FILES[@]}"; do
  src_file="$SRC/$rel"
  dst_file="$DST/$rel"

  if [[ ! -f "$src_file" ]]; then
    echo "skip  $rel  (not in source)"
    ((missing++)) || true
    continue
  fi

  if [[ -f "$dst_file" ]] && cmp -s "$src_file" "$dst_file"; then
    echo "ok    $rel  (already in sync)"
    ((skipped++)) || true
    continue
  fi

  mkdir -p "$(dirname "$dst_file")"
  cp -p "$src_file" "$dst_file"
  echo "copy  $rel"
  ((copied++)) || true
done

echo
echo "done: $copied copied, $skipped already in sync, $missing missing in source"

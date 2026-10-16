#!/usr/bin/env bash
# Sets a new version in VERSION, commits it as v<version> with a tag of the
# same name, and after asking pushes main and the tag, which makes the
# Release workflow build, sign, notarize and publish it.
# Usage: Scripts/bump.sh <version | patch | minor | major>
#
# The "## Unreleased" section of CHANGELOG.md becomes "## <version> - <date>"
# in the same commit: the update window shows it, it is the notes of the
# GitHub release, and the release fails without it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ $# -ne 1 ]; then
    sed -n '2,9s/^# \{0,1\}//p' "$0" >&2
    exit 1
fi

cd "$ROOT"
CURRENT="$(tr -d '[:space:]' < VERSION)"
# Prints the new version, or fails when the argument isn't a version or
# keyword or doesn't come after the current version.
VERSION="$(/usr/bin/python3 - "$CURRENT" "$1" <<'PY'
import re, sys

def parse(v):
    m = re.fullmatch(r"(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)", v)
    if not m:
        sys.exit(f"{v} isn't a version such as 1.2.0.")
    return tuple(int(x) for x in m.groups())

current, arg = parse(sys.argv[1]), sys.argv[2]
major, minor, patch = current
new = {
    "patch": (major, minor, patch + 1),
    "minor": (major, minor + 1, 0),
    "major": (major + 1, 0, 0),
}.get(arg) or parse(arg)
if new <= current:
    sys.exit("%d.%d.%d doesn't come after the current version %s." % (*new, sys.argv[1]))
print("%d.%d.%d" % new)
PY
)"
TAG="v$VERSION"

if [ "$(git symbolic-ref --short HEAD 2>/dev/null)" != main ]; then
    echo "Releases are made from main. Switch to it first." >&2
    exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
    echo "The working tree has changes. Commit them first." >&2
    exit 1
fi
git fetch --quiet --tags origin main
if [ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]; then
    echo "main differs from origin/main. Pull or push first." >&2
    exit 1
fi
if git rev-parse --verify --quiet "refs/tags/$TAG" >/dev/null \
    || [ -n "$(git ls-remote --tags origin "refs/tags/$TAG")" ]; then
    echo "Tag $TAG exists already." >&2
    exit 1
fi
if ! /usr/bin/python3 - <<'PY'
import re, sys
text = open("CHANGELOG.md").read()
m = re.search(r"^## Unreleased\n(.*?)(?=^## |\Z)", text, re.M | re.S)
sys.exit(0 if m and m.group(1).strip() else 1)
PY
then
    echo "CHANGELOG.md has no \"## Unreleased\" section with notes. Add them and commit first." >&2
    exit 1
fi

echo "==> $CURRENT -> $VERSION"
printf '%s\n' "$VERSION" > VERSION
/usr/bin/python3 - "$VERSION" <<'PY'
import re, sys, datetime
with open("CHANGELOG.md") as f:
    text = f.read()
day = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d")
text = re.sub(r"^## Unreleased$", "## Unreleased\n\n\n## %s - %s" % (sys.argv[1], day), text, count=1, flags=re.M)
with open("CHANGELOG.md", "w") as f:
    f.write(text)
PY
if [ "$(git diff --name-only | tr '\n' ' ')" != "CHANGELOG.md VERSION " ]; then
    echo "Expected only CHANGELOG.md and VERSION to change:" >&2
    git diff >&2
    echo "Undo with: git checkout -- CHANGELOG.md VERSION" >&2
    exit 1
fi
git commit --quiet -m "release: $VERSION" CHANGELOG.md VERSION
git tag "$TAG"
git --no-pager log -1 --stat --format='%h %s' "$TAG"
echo
echo "==> notes of the release, from CHANGELOG.md:"
Scripts/release-notes.sh "$VERSION"
echo

read -r -p "Push main and $TAG to origin, which publishes the release? [y/N] " answer || echo
if [[ "$answer" != [yY]* ]]; then
    echo "Not pushed. Push later with: git push --atomic origin main $TAG"
    echo "Or undo with: git tag -d $TAG && git reset --hard HEAD~1"
    exit 0
fi
git push --quiet --atomic origin main "$TAG"
echo "==> pushed $TAG. The Release workflow: https://github.com/sorrycc/shun/actions/workflows/release.yml"

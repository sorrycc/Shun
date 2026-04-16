# Shun

macOS 14+ menu bar app for global hotkey-to-app switching. (瞬 — "instant".)

## Build

```bash
swift build                  # debug
swift build -c release       # release
./Scripts/package_app.sh     # bundle Shun.app (ad-hoc signed)
```

Then `cp -r Shun.app /Applications/`.

## Docs

- `CLAUDE.md` — architecture & conventions
- `docs/designs/` — design specs (still use the old name "HotApp")

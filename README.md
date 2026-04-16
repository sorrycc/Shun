# Shun

macOS 14+ menu bar app for global hotkey-to-app switching. (瞬 — "instant".)

## Screenshots

![Shun screenshot 1](https://pic.sorrycc.com/proxy/1776356535786-85374041.png)
![Shun screenshot 2](https://pic.sorrycc.com/proxy/1776356593804-890547415.png)

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

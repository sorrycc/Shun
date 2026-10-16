# Shun

macOS 14+ menu bar app for global hotkey-to-app switching. (瞬 — "instant".)

A shortcut brings its app to the front, launching it when it isn't running, and hides it when it is in front already. Shun takes the keys with an event tap and switches apps in well under a millisecond.

Written in Go, with a native UI drawn by [MyGo](https://github.com/egoist/mygo): no cgo, no webview, a single binary.

## Screenshots

![Shun screenshot 1](https://pic.sorrycc.com/proxy/1776356535786-85374041.png)
![Shun screenshot 2](https://pic.sorrycc.com/proxy/1776356593804-890547415.png)

## Build

Needs Go 1.27+ (and Bun, which evaluates `mygo.config.ts`).

```bash
Scripts/dev.sh                                # run "Shun Dev" with your shortcuts, rebuilt as you save
go test ./...                                 # tests, the Settings window's included, headless
go tool mygo build -platform darwin/arm64     # build/darwin-arm64/Shun.app and a .dmg (ad hoc signed)
```

Then `cp -r build/darwin-arm64/Shun.app /Applications/`. Shun needs the Accessibility permission to take the keys
(System Settings → Privacy & Security → Accessibility); until it has it, shortcuts still work, more slowly, through
global hotkeys.

## Docs

- `CLAUDE.md` — architecture & conventions
- `CONTRIBUTING.md` — releasing
- `docs/designs/` — design specs of the Swift versions (still use the old name "HotApp")

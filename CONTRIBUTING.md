# Contributing

## Logs

The app logs via `os.Logger` under subsystem `com.shun`.

Live tail:
```bash
log stream --predicate 'subsystem == "com.shun"' --level debug
```

Past logs (last hour, including `.debug`/`.info`):
```bash
log show --predicate 'subsystem == "com.shun"' --last 1h --info --debug
```

Console.app: select your Mac in the sidebar, then filter `subsystem:com.shun`.

Narrow by category (e.g. `ShortcutManager`, `ShortcutRow`):
```bash
log stream --predicate 'subsystem == "com.shun" AND category == "ShortcutManager"' --level debug
```

# Shun — Brainstormed Ideas (v2)
_Generated 2026-04-18 via 8 parallel agents across distinct expert lenses (Power User, macOS Platform, AI/ML, UX, Competitive, Extensibility, Accessibility, Growth/Business). Continues the numbering from `2026-03-18-brainstorm.md` (ideas #1–#12). Duplicates surfaced across lenses (Leader-Key, CLI companion) have been merged._

Guiding question: _"What's the single smartest and most radically innovative and accretive and useful and compelling addition we could make to Shun at this point?"_

---

## 13. Leader-Key Modal Mode (Vim/which-key for your Mac)
**Lens:** Power User / Competitive | **Effort:** Medium

One single leader hotkey (e.g. `⌘Space`) enters a modal "command mode" with a heads-up which-key-style overlay showing available next-key bindings. `⌘Space → w → t` jumps to Terminal in Work context; `⌘Space → s` launches Slack. Unlimited bindings with zero modifier-combo collision — and the overlay teaches itself.

- New `LeaderTree: Codable` — trie of key → (action | submenu) with per-node labels
- Modal state machine in `ShortcutManager`: after leader press, arm `CGEventTap` / local `NSEvent` monitor for next keystrokes, timeout after 1.5s, bail on Esc
- Translucent which-key `NSPanel` overlay lists live bindings, auto-hides on resolve; character `瞬` micro-etch
- Migration: existing flat `AppShortcut` entries auto-import as single-depth leaves
- Extend `AppShortcut` with `leaderPath: [Character]?` (nil = traditional global combo)

**Why it's compelling:** Traditional hotkey apps hit a ceiling around 10 `⌘⌥⇧` combos before brain and keyboard collide. Leader mode gives users literally the entire alphabet as bindings, with zero conflicts, mirrored from Vim/tmux/Spacemacs workflows power users already love. No competitor (Thor, Raycast, Alfred) has a modal/leader UX; copying it requires rethinking their entire input model.

---

## 14. Shun CLI Companion (`shun` in PATH)
**Lens:** Power User / Extensibility | **Effort:** Low-Medium

A tiny Swift CLI binary shipped inside the app bundle and symlinked to `/usr/local/bin/shun` on first launch (with consent). Gives scripts, `.zshrc` aliases, git hooks, tmux statuslines, and build pipelines a zero-dependency way to drive Shun: `shun switch com.apple.Terminal`, `shun list --json | jq`, `shun bind t com.apple.Terminal --leader`, `shun current`, `shun disable --for 25m`.

- Single-file `Sources/shun-cli/main.swift` SPM target; bundle binary into `Shun.app/Contents/MacOS/shun`
- Talk to running app over a Unix domain socket at `~/Library/Application Support/Shun/cli.sock` (JSON-lines protocol, no TCP/auth surface — filesystem perms only)
- First-run Settings prompt offers "Install `shun` command" → `/usr/local/bin/shun` symlink via Authorization Services escalation
- Subcommands: `switch`, `toggle`, `list`, `bind`, `unbind`, `current`, `enable`, `disable`, `reload`, `mcp` (see #27), all with `--json`
- Shell completions: `shun completions zsh|bash|fish`

**Why it's compelling:** CLI presence is the cheapest legitimacy signal a Mac app can buy — how Raycast, 1Password, and VS Code earned developer mindshare. Unlocks every existing automation surface (Makefiles, git hooks, Hammerspoon, Alfred) without Shun writing a single integration. Strictly better than the HTTP hook idea (#12) for keyboard-first users who already live in shells.

---

## 15. Git/Repo-Aware Smart Targets
**Lens:** Power User | **Effort:** Medium

A shortcut's target can be a **dynamic rule**, not just a static bundle. Bind `⌘⌥T` → "Terminal, `cd`'d to the repo of whatever editor window is frontmost." Bind `⌘⌥E` → "Editor registered for the current repo's primary language." Shun resolves the target at press time using the frontmost window's document URL + a user-configurable resolver chain.

- Extend `AppShortcut` with `resolver: TargetResolver` enum: `.static(bundleID)`, `.repoTerminal`, `.repoEditor`, `.siblingFolder`, `.script(path)`
- Use Accessibility API (`AXUIElement`) to read frontmost window's document URL; walk upward for `.git`
- Terminal resolvers emit AppleScript/ObjC automation to open a new tab `cd`'d to the repo root — one small adapter per terminal (Terminal, iTerm2, Ghostty, WezTerm)
- `.script(path)` resolver shells out to a user script that prints a bundle ID on stdout — ultimate escape hatch

**Why it's compelling:** Every existing switcher treats apps as static targets. Context-resolved targets collapse the "switch to Terminal, then `cd ~/code/foo`" two-step into one keypress — mirroring how developers actually think ("I want a shell for *this* project") rather than how macOS models apps.

---

## 16. Shortcuts.app Action Provider (App Intents)
**Lens:** macOS Platform | **Effort:** Medium

Ship an App Intents extension that exposes every Shun shortcut as a first-class action in Apple's Shortcuts.app. "Switch to App", "Toggle Global Enable", "Activate Workspace", "Set Context" become draggable blocks users chain with Calendar triggers, Stream Deck, keyboard Quick Actions, or the Action Button on a paired iPhone via Continuity.

- New `AppIntents` target with `SwitchToAppIntent`, `ToggleShunIntent`, `ActivateContextIntent` conforming to `AppIntent`
- Expose `AppShortcut` instances via `AppEntityQuery` so Shortcuts.app autocompletes the user's actual configured apps
- Route intent `perform()` into existing `ShortcutManager` via shared XPC or `NSDistributedNotification`
- Register an `AppShortcutsProvider` so Siri/Spotlight phrases work automatically (modern successor to #6's `NSUserActivity` approach)

**Why it's compelling:** Turns Shun into a composable automation primitive — users bridge it to HomeKit, Calendar, Focus filters, and the iPhone Action Button without code. App Intents is the Apple-blessed successor to legacy intent APIs and unlocks iPadOS reach if Shun ever expands.

---

## 17. Space & Display-Aware Shortcut Scopes
**Lens:** macOS Platform | **Effort:** Medium-High

Each shortcut can be scoped to a specific Mission Control Space, Stage Manager set, or physical display — so `⌘1` opens Terminal on your coding Space but Messages on your "communication" Space, and the laptop-only Space never triggers the Studio Display's dev tools. Shun listens for Space/display transitions and re-registers within milliseconds.

- Observe `NSWorkspace.activeSpaceDidChangeNotification` plus `CGSCopyManagedDisplaySpaces` (private but stable) for current Space UUID
- Track `NSApplication.didChangeScreenParametersNotification` + `NSScreen.screens` for display-scoped rules
- Extend `AppShortcut` with `scope: {spaces: [UUID]?, displays: [CGDirectDisplayID]?, stageManagerSet: String?}`
- On scope change, diff current vs. desired registrations and call `ShortcutManager.refreshRegistrations()` — same mechanism as #5 (Focus Mode) but orthogonal

**Why it's compelling:** Spatial context is how power users already organize work — Shun is the only app that can make the keyboard layer match the spatial layer. No competitor understands Spaces, and Stage Manager desperately needs third-party glue.

---

## 18. Accessibility-Powered Window Cycling & Peek
**Lens:** macOS Platform | **Effort:** Medium

When an app has multiple windows (Chrome profiles, Terminal tabs, several Finder windows), pressing the hotkey repeatedly cycles windows *within* that app. Hold the hotkey for 400ms to reveal a translucent "peek" HUD with window titles and live thumbnails; arrow-keys to select, release to activate.

- Request Accessibility permission, wrap target app in `AXUIElementCreateApplication`, enumerate `kAXWindowsAttribute`
- On repeated press within 600ms, raise next `AXWindow` via `AXUIElementSetAttributeValue(kAXMainAttribute)` instead of re-toggling
- Peek HUD as borderless `NSPanel` at `.floating` level, titles from `CGWindowListCopyWindowInfo`, live previews from `CGWindowListCreateImage` (sub-10ms)
- Graceful degrade to today's app-toggle if Accessibility is denied; one-tap permission prompt in Settings

**Why it's compelling:** Solves the #1 Thor/HotApp complaint — "I have 4 Chrome windows and I can't get to the right one." Peek turns Shun into a tactile window navigator rivaling macOS's own `⌘`` ` but scoped to any hotkey.

---

## 19. Foundation Models Window-Title Triage
**Lens:** AI/ML | **Effort:** Medium

When a hotkey fires, pass the target app's visible window titles through Apple's on-device Foundation Models framework (macOS 15.1+) to pick the *right* window — not just the frontmost one. `⌘M` for Mail activates the compose window mid-draft over the inbox; `⌘T` for Terminal surfaces the tab running your dev server over an idle shell.

- Accessibility API (`AXUIElementCopyAttributeValue` with `kAXWindowsAttribute`) to enumerate windows + titles
- Prompt `SystemLanguageModel` with a guided-generation schema: `{windowIndex: Int, reason: String}` given titles + recent user intent
- Cache per-(bundleID, title-hash) decisions for 60s to keep p95 activation <50ms; fall back to frontmost on model unavailability
- New `WindowRouter` service behind feature flag in Settings → Advanced

**Why it's compelling:** Hotkey switchers universally cheat by activating "the app" — users still hunt for the right window. Title-aware routing turns one hotkey into a semantic destination, zero cloud, zero user configuration.

---

## 20. Ambient Context Sensor ("You look busy")
**Lens:** AI/ML | **Effort:** Medium-High

A lightweight on-device signal-fusion layer watches EventKit (active meeting?), `CGWindowListCopyWindowInfo` (screen-share or fullscreen video?), and mic input level (speaking?), then gates or rewrites hotkey behavior. Hitting Slack hotkey during Zoom routes to "set status: in a meeting" instead of stealing focus; hitting it when the calendar is clear behaves normally.

- New `AmbientContext` actor publishing `@Observable` enum: `.meeting`, `.presenting`, `.focused`, `.idle`
- Per-shortcut override rules: `behavior: [ContextState: Action]` (activate / suppress / script / notify)
- Anomaly detection on usage log: k-means over (hour, weekday, activeApp) flags "you usually switch to Figma at 2pm — still heads down?" as a menu bar nudge
- All processing in-process; EventKit prompt gated behind one-time opt-in

**Why it's compelling:** Transforms Shun from a passive dispatcher into a context-aware agent that *declines* to switch when switching would be destructive. No competitor understands that a hotkey press during screen-share has different intent than the same press 10 seconds later.

---

## 21. Semantic App Aliases via On-Device Embeddings
**Lens:** AI/ML | **Effort:** Low

At app-scan time, generate a ~384-dim embedding per installed app using `NaturalLanguage.NLContextualEmbedding` (macOS 14+) over `{displayName, CFBundleIdentifier, LSApplicationCategoryType, Info.plist description}`. When searching in Settings or the Command Palette (#3), rank by cosine similarity, not substring match. Typing "draw" finds Figma, Sketch, Procreate; "notes" finds Bear, Obsidian, Apple Notes.

- `EmbeddingIndex` service: persist `[bundleID: [Float]]` to Application Support as flat binary, rebuild on AppScanner cache invalidation
- Query-time: embed the search string, cosine-rank top-20 against cached vectors (<2ms for a few hundred apps)
- Hybrid ranking: blend semantic score with usage-frequency boost from Idea #4
- Graceful fallback to substring matcher on macOS <14

**Why it's compelling:** Users' mental model ("the thing I draw in") rarely matches the marketing name. Embeddings are free, tiny, and already shipped by Apple — the lowest-effort "feels like magic" AI feature in the whole backlog, elevating every search surface.

---

## 22. First-Launch "Hello" Choreography
**Lens:** UX Design | **Effort:** Low-Medium

A 20-second first-run moment that teaches the core gesture before asking anything. After permissions, the menu bar icon pulses once, a labeled arrow points at it ("Hi. I live up here."), then a translucent HUD shows three suggested starter shortcuts pre-paired with apps already in the user's Dock — accept each with Return. No forms, no tabs, no empty state.

- New `OnboardingCoordinator` (`NSPanel` overlay) triggered when `shortcuts.json` is empty AND accessibility was newly granted
- `NSStatusItem` button animates via `CABasicAnimation` on its view layer + one-shot popover with upward tail
- Dock detection: read `~/Library/Preferences/com.apple.dock.plist` `persistent-apps` to suggest 3 plausible pairings (⌘⌥1/2/3)
- "Skip" always visible; state stored in UserDefaults so it never re-appears

**Why it's compelling:** LSUIElement apps are notoriously invisible post-install — users quit them because "nothing happened." Choreographing the first 20 seconds converts curiosity into muscle memory before the user forgets the app exists.

---

## 23. Shun HUD — The Centered Keystroke Moment
**Lens:** UX Design | **Effort:** Medium

Every hotkey press briefly summons a centered, screen-filling-but-minimal HUD: a large app icon (~128pt) + app name in SF Pro Display on a gaussian-blurred vibrancy background. Fades in 80ms, out 180ms. The character `瞬` micro-etches below on first daily use. Optional tuned tick at -24dB. "Minimal / Classic / Zen / Silent" presets.

- `HUDController: NSWindowController` backed by borderless `.nonactivatingPanel` + `.floatingPanel`, `ignoresMouseEvents = true`
- SwiftUI: `AsyncImage`-loaded icon + `.transition(.opacity.combined(with: .scale(0.96)))`
- Hook into `ShortcutManager.performToggle()` — fire-and-forget call, auto-dismiss via `Task.sleep`
- Preference pane: preset picker + live preview + "on activation / only on launch / never"

**Why it's compelling:** The core action currently has *zero* visual acknowledgement — users fat-finger combos and can't tell what happened. A brief, beautiful HUD turns every press into a tiny ceremony and gives the app a visible identity. It's the detail that turns utilities into keepsakes (Keyboard Maestro's HUD, Raycast's launcher).

---

## 24. Living Menu Bar Icon
**Lens:** UX Design | **Effort:** Low

The menu bar icon gains subtle state-aware personality via SF Symbols variable rendering. Each hotkey press triggers a crisp 120ms "tick" scale (1.0 → 0.88 → 1.0). When globally disabled, fades to 40% opacity with a hairline slash. During scheduled silence (pairs with #2), dims with a tiny clock overlay. On new-version-available (Sparkle), gains a 4pt accent dot. Opt-in birthday variant (from Contacts "me" card): confetti for one day.

- Replace current template image with SF Symbols composites rendered at 18pt via `NSImage.SymbolConfiguration` with `.hierarchical`
- Animate via `NSStatusItem.button.layer` with `CASpringAnimation` (duration 0.12, damping 12)
- Add `StatusIconState` enum (`.idle / .active / .disabled / .silenced / .updateAvailable / .celebratory`) owned by `StatusItemController`
- Wire state transitions to existing `.shortcutsDidChange` + `UpdaterService` publishers

**Why it's compelling:** The menu bar icon is the app's entire surface area for 99% of sessions. A living pulse makes Shun feel *present* without being noisy; rare celebratory states create emotional hooks — the exact mechanism that made Tot, Bartender, and Things feel beloved rather than merely functional.

---

## 25. Spatial Hotkeys (Per-Display Geometry Bindings)
**Lens:** Competitive | **Effort:** Medium-High

Hotkeys bind not just to an app but to a `(display, region, app, window-state)` tuple. `⌘1` activates Slack on the laptop screen left-half; the same `⌘1` activates Slack full-screen on the external 4K. Shun owns both switching AND window placement in one keypress — collapsing Rectangle/Magnet + Thor into one app.

- Extend `AppShortcut` with optional `WindowTarget: { displayUUID?, frame: CGRect-relative, windowIndex?, fullscreen }`
- Use `CGDisplay` APIs + Accessibility (AXUIElement) to move/resize the frontmost window on activation
- Display-aware resolver: on keypress, pick binding whose `displayUUID` matches the current mouse/focus display; fall back to "any"
- UI: visual display picker (mini monitor diagram) + region grid (halves/thirds/quarters) per shortcut

**Why it's compelling:** Multi-monitor users currently stitch together 3 apps (Thor + Rectangle + BetterTouchTool). Nobody ships "switch AND place" as an atomic action. Raycast explicitly punts window management to extensions — this is a legitimate product category they can't cheaply copy. Orthogonal to #17 (scope filtering); this is scope *execution*.

---

## 26. Smart Alt-Tab Cycling (Recency + Intent)
**Lens:** Competitive | **Effort:** Low-Medium

Press-and-hold the shortcut to cycle: tap `⌘1` = toggle Terminal; hold `⌘1` = float an MRU ribbon of your last 5 terminals/IDEs/browsers, tab through with the same key. Unlike `⌘Tab` (global, chronological, ugly) or `⌘\`` (per-app, no preview), Shun cycles within a semantic group the user defined, with live window thumbnails.

- Extend `AppShortcut` → `cycleGroup: [bundleID]` (e.g. "browsers" = Chrome, Safari, Arc)
- Detect key-hold via `flagsChanged` / `keyUp` delta >200ms; first tap = toggle, hold = ribbon
- Lightweight `NSPanel` ribbon with `CGWindowListCopyWindowInfo` + cached thumbnails via `CGWindowListCreateImage`, arrow/tap-same-key to advance
- MRU state per group persisted in memory, promoted on each activation

**Why it's compelling:** `⌘Tab` hasn't evolved since 2001; every power user has a mental model of "my browsers" vs. "my editors" that the OS refuses to understand. Shipping this makes Shun the first hotkey app that *replaces* `⌘Tab` — not augments it. Demo-able, screenshot-friendly, HN-front-page bait ("I rebuilt `⌘Tab` for 2026").

---

## 27. MCP Server for AI Assistants
**Lens:** Extensibility | **Effort:** Medium

Ship an embedded Model Context Protocol server so Claude Desktop, Cursor, and Zed agents can read and mutate Shun state as first-class tools: `shun.list_shortcuts`, `shun.activate_app`, `shun.register_ephemeral`, `shun.get_active_context`. Exposed over stdio via `shun mcp` (using the CLI from #14) — one line in `claude_desktop_config.json`.

- New `Sources/ShunMCP/` target implementing MCP 2025-06 spec over stdio (JSON-RPC 2.0)
- Tool surface mirrors CRUD + activation; guarded by per-client consent prompt in Settings → "Connected Agents"
- Resources endpoint exposes live shortcut list + usage log as read-only resources for agent reasoning
- Reuses same Unix socket bridge from the CLI (#14) to reach the running app — no second event loop

**Why it's compelling:** Every hotkey Shun owns becomes a verb your AI agent can invoke — "Claude, put me in Writing context and launch Bear" actually works. Being the first hotkey manager with MCP puts Shun on the short list of agent-native Mac apps for the next 12 months.

---

## 28. Shunscript — Embedded Lua Triggers
**Lens:** Extensibility | **Effort:** Medium-High

Optional Lua scripts attached to any shortcut as `onPress` / `onRelease` / `onDoublePress` hooks, running in a sandboxed embedded Lua 5.4 VM. Thirty lines of Lua can replace what today requires Hammerspoon + Keyboard Maestro + AppleScript: detect frontmost app, cycle windows, post a notification, fire a webhook, or chain into another shortcut.

- Embed Lua via SwiftPM wrapper (~200KB); expose a curated `shun.*` API (activateApp, frontmost, notify, httpPost, sleep, getContext)
- Per-script timeout (250ms default) enforced on a background actor; script failures surface as non-blocking toast
- Scripts live in `~/Library/Application Support/Shun/scripts/<uuid>.lua` — plain text, version-controllable, shareable via Recipes (#11)
- Settings editor is a plain `TextEditor` with TreeSitter-lua syntax highlight + "Test Run" button

**Why it's compelling:** Scripting is the moat that turns power users into evangelists — Hammerspoon's community survives on 50-line gists. Lua (not JS) keeps the sandbox trivial and the binary small, while giving Shun a plugin substrate that every other idea (recipes, webhooks, workspaces) can eventually compile down to.

---

## 29. IME-Aware Hotkey Suspension
**Lens:** Accessibility | **Effort:** Low

Chinese/Japanese/Korean IME users lose hotkeys to composition buffers — pressing ⌘Space mid-pinyin either fires the app switch or eats the character, never predictably. Shun detects active IME composition and briefly suspends conflicting hotkeys, restoring them the instant composition commits or cancels.

- Observe `TISCopyCurrentKeyboardInputSource()` + `kTISPropertyInputSourceCategory` for non-Roman input sources at registration time
- Hook distributed notification `com.apple.Carbon.TISNotifySelectedKeyboardInputSourceChanged` and inspect frontmost app's marked-text state via AX API to gate `ShortcutManager.refreshRegistrations()`
- New per-shortcut toggle "Suspend during IME composition" (default on for single-modifier combos, off for `⌘⌥`/`⌘⌥⇧`)
- Visual badge in `ShortcutListView` when a combo conflicts with the currently-selected IME (e.g. ⌃Space vs. Pinyin candidates)

**Why it's compelling:** CJK users are ~1.5B people globally and every Thor-clone ignores them — the lowest-effort way to open Shun to a market no competitor serves.

---

## 30. Dwell & Sequential-Chord Triggers (Native Sticky Keys)
**Lens:** Accessibility | **Effort:** Medium

Users with RSI, tremor, or one-handed operation physically cannot hold `⌘⌥⇧L` simultaneously — macOS Sticky Keys helps but breaks on third-party hotkey libraries. Shun adds first-class sequential-chord mode (press modifiers one at a time, each latches for N ms) plus dwell mode (holding a single key for a duration fires the shortcut). Every combo becomes one-finger reachable.

- Replace per-shortcut binding with a `TriggerMode` enum: `.simultaneous` (current), `.sequential(latchMs: Int)`, `.dwell(keyCode: Int, holdMs: Int)`
- Sequential latching via `CGEventTap` at `.cgAnnotatedSessionEventTap` — track modifier-down events, hold latch state 800ms (tunable), fire on final key
- Dwell detection: single-key down → `DispatchSourceTimer` at holdMs; cancel on key-up or any other key; emit synthetic event on fire
- Settings pane "Accessibility" with latch/dwell sliders, live preview, respects `AXUIElementCopyAttributeValue(kAXEnhancedUserInterface)`

**Why it's compelling:** Converts a power-user tool into assistive tech without forking the hotkey engine — and the same plumbing enables foot-pedal / AAC-switch integration (HID device posting a single keycode) for sip-and-puff users.

---

## 31. VoiceOver-First Menubar Surface
**Lens:** Accessibility | **Effort:** Low-Medium

An `LSUIElement` app is literally invisible to blind users — no Dock icon, and `NSStatusItem` rotor navigation in VoiceOver is notoriously broken. Shun exposes a dedicated "Shortcuts" rotor group, speaks combos with full modifier names (`Command Option L`, not `⌘⌥L`), and renders lists with WCAG-AAA contrast, Atkinson Hyperlegible type, and numeric-glyph disambiguation.

- `setAccessibilityRole(.menuBar)` + custom `accessibilityLabel` on `NSStatusItem.button` with spoken summary ("Shun, 7 shortcuts active, Command Option H to open palette")
- Register `NSAccessibilityCustomRotor` named "Shun Shortcuts" so VO users hit `VO+U` anywhere and jump to the list
- `ShortcutListView` gains `.highContrast` environment: 7:1 ratio, 16pt minimum, Atkinson Hyperlegible bundled as optional font, combos rendered as words next to glyphs
- Honor `NSWorkspace.shared.accessibilityDisplay…` flags (IncreaseContrast, ReduceMotion, DifferentiateWithoutColor drives icon *shape* variation, not just tint)

**Why it's compelling:** Menubar apps are a known VoiceOver dead zone — shipping a genuinely navigable one is a rare category-defining move. The high-contrast/dyslexia work doubles as a polish win for every sighted user in bright sunlight.

---

## 32. keymap.dev — Public Profiles for Your Keyboard
**Lens:** Growth | **Effort:** High

Every Shun user gets a vanity URL (`keymap.dev/sorrycc`) showcasing their hotkey setup as a beautiful, scannable keyboard diagram — live SVG of a Mac keyboard with colored overlays mapping each combo to app icons. Visitors hit "Clone this keymap" → `shun://install?profile=sorrycc` imports the full set. Dotfiles culture, but for muscle memory.

- Static site generator baked into Shun: "Publish profile" → uploads JSON + rendered SVG/OG-image to R2-backed profiles service (reuse existing R2 bucket)
- `shun://` URL scheme handler in `AppDelegate` with fuzzy bundle-ID matching (reuses Recipe model from #11) + confirm-before-overwrite sheet
- Auto-generated OG images so every shared link previews gorgeously on Twitter/LinkedIn/Slack
- Optional GitHub login for claiming the handle; profile shows install count, fork count, "used by N people" social proof

**Why it's compelling:** Converts private config into public identity — the single strongest growth vector in developer tools (dotfiles, Raycast extensions, VS Code themes all proved this). Every power user becomes a distribution channel; every cloned keymap is a signed-in new user.

---

## 33. Hotkey Wrapped — Your Year in Keystrokes
**Lens:** Growth / Loyalty | **Effort:** Medium

Every December, Shun generates a Spotify-Wrapped-style animated recap: "You pressed ⌘⌥T 4,812 times. Your most-loved app was Ghostty. You saved 14 hours vs. clicking the Dock. You're in the top 3% of Terminal switchers." Swipeable card stack, exported as a square PNG/video optimized for Twitter/LinkedIn/Threads. Drops Dec 1 with an in-app notification.

- Builds on Usage Insights (#4) — already-logged `UsageLog` provides raw data, zero new telemetry
- `WrappedView` SwiftUI scene with Canvas-based animated cards (typography-driven, 9:16 + 1:1 export modes)
- Percentile stats require anonymous opt-in aggregate telemetry (one POST of bucketed counts → R2 + tiny Worker), clearly disclosed
- Seeded "share to X / LinkedIn / Threads" buttons with pre-filled copy linking to `shun.app` + watermark on exported image

**Why it's compelling:** An annual, time-boxed viral moment every tech-Twitter user understands on sight. Turns a silent utility into a yearly cultural event; screenshots do the marketing for free. Spotify proved this is the highest-ROI growth mechanic ever invented for a consumable product.

---

## 34. Shun Teams — Shared Keymaps for Engineering Orgs
**Lens:** Business | **Effort:** High

A paid tier ($4/seat/mo or $40/seat/yr) where an engineering manager defines a team keymap once — "at Acme, ⌘⌥1 is always Linear, ⌘⌥2 is always Datadog" — and every new hire's Shun auto-syncs on first login. When the team adopts a new tool (Cursor replaces VS Code), one admin change rolls out to 80 devs overnight. Core app stays free forever; Teams is purely org-level infrastructure.

- Sign in with Apple on the desktop client; team membership resolved via a lightweight Cloudflare Workers + D1 backend (stateless, cheap, already in R2 ecosystem)
- New "Managed" section in Settings showing team-pushed shortcuts (locked or overrideable, admin's choice); local shortcuts preserved underneath
- Admin web dashboard at `shun.app/teams` for keymap editing, onboarding-link generation, and per-team usage heatmaps
- Stripe checkout; free for teams ≤3 so indie duos/trios stay free and bring it to their next employer

**Why it's compelling:** Shun is already the exact shape of a bottom-up SaaS wedge — individual devs love it, then bring it to work. Teams solves a real onboarding pain (new hires spend weeks learning which hotkey launches what) while giving Shun durable revenue without paywalling the individual experience. Every enterprise adoption seeds 50+ new consumer users.

---

## Priority Matrix

| # | Idea | Effort | Impact | Recommendation |
|---|------|--------|--------|----------------|
| 21 | Semantic App Aliases (embeddings) | Low | ★★★★☆ | **Ship first — lowest effort / magic ratio in the backlog** |
| 14 | Shun CLI (`shun` in PATH) | Low-Med | ★★★★★ | **Ship first — unlocks #27 MCP + every script** |
| 24 | Living Menu Bar Icon | Low | ★★★☆☆ | Low-hanging polish |
| 29 | IME-Aware Suspension | Low | ★★★★☆ | Low-hanging & opens CJK market |
| 22 | First-Launch Choreography | Low-Med | ★★★★☆ | Convert installs to habit |
| 23 | Shun HUD | Medium | ★★★★☆ | Gives the app a face |
| 13 | Leader-Key Modal Mode | Medium | ★★★★★ | Signature feature; defines category |
| 18 | Window Cycling & Peek | Medium | ★★★★★ | Fixes the #1 Thor complaint |
| 26 | Smart Alt-Tab Cycling | Low-Med | ★★★★☆ | HN-frontpage demo candidate |
| 15 | Git/Repo-Aware Smart Targets | Medium | ★★★★☆ | Developer love, deep moat |
| 16 | App Intents / Shortcuts.app | Medium | ★★★★☆ | Apple-blessed reach |
| 19 | Foundation Models Window Triage | Medium | ★★★★☆ | Build after #18 |
| 20 | Ambient Context Sensor | Med-High | ★★★★☆ | Build after #4 Usage Log |
| 27 | MCP Server for AI Assistants | Medium | ★★★★☆ | Ride the 2026 agent wave |
| 17 | Space & Display-Aware Scopes | Med-High | ★★★★☆ | Pairs with #5 Focus Modes |
| 25 | Spatial Hotkeys (geometry) | Med-High | ★★★★★ | Absorbs Rectangle/Magnet market |
| 32 | keymap.dev Profiles | High | ★★★★★ | Growth flywheel |
| 33 | Hotkey Wrapped | Medium | ★★★★☆ | Annual viral moment (needs #4) |
| 30 | Dwell/Sequential Chords | Medium | ★★★☆☆ | Accessibility + pedal users |
| 31 | VoiceOver-First Surface | Low-Med | ★★★☆☆ | Category-defining on accessibility |
| 28 | Shunscript (Lua) | Med-High | ★★★★☆ | Platform play; plugin substrate |
| 34 | Shun Teams (paid tier) | High | ★★★★★ | Durable revenue; ship last |

### Suggested Waves

**Wave A (next 4–6 weeks) — polish + platform primitives:**
#21 Embeddings → #14 CLI → #24 Living Icon → #23 HUD → #22 Onboarding → #29 IME.
These are low-risk, mostly Low-Medium effort, and most of them light up every other idea downstream.

**Wave B (category-defining features):**
#13 Leader Keys → #18 Window Peek → #26 Smart Alt-Tab → #15 Repo-Aware Targets.
The four features that make HN and turn a utility into a daily-driver.

**Wave C (platform expansion):**
#16 App Intents → #27 MCP → #17 Spaces/Display → #25 Spatial Hotkeys → #28 Shunscript.
Positions Shun as the Mac's keyboard-layer platform for the agent era.

**Wave D (growth & revenue):**
#32 keymap.dev → #33 Hotkey Wrapped → #34 Shun Teams.
Only after the core is beloved; these turn love into distribution and revenue.

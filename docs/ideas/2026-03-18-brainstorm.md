# HotApp — Brainstormed Ideas
_Generated 2026-03-18 via 6 parallel agents across different expert lenses_

---

## 1. Smart Workspaces (App Bundles)
**Lens:** Power User | **Effort:** Medium

A single hotkey activates a whole constellation of apps. Press `⌘⌥W` → Terminal, VS Code, Finder, Slack all launch/activate in sequence with configurable inter-activation delays (~100ms each).

- Extend `AppShortcut` to optionally reference a `[UUID]` bundle of child apps
- New `AppBundle: Codable` model with name, ordered steps, per-step delay
- Execution: loop + `Task.sleep` between activations in `ShortcutManager`
- UI: new "Workspaces" tab reusing `AppSearchView` for multi-select

**Why it's compelling:** Transforms HotApp from "single-app switcher" to workflow orchestrator. Thor never had this. Every developer has a "morning ritual" sequence they do manually today.

---

## 2. Scheduled Hotkey Silencing (Focus Blocks)
**Lens:** Power User | **Effort:** Medium-High

Time-based rules mute specific hotkeys on a schedule. "Disable Slack/Mail shortcuts weekdays 9-10am." Rules are recurring (days-of-week + time range), app-specific or global, and override the global enable toggle.

- `ScheduleRule: Codable` with recurring spec, app filter, start/end time
- On keypress, check current time against rules before dispatching toggle
- UI: simple rules editor (time picker + weekday checkboxes + app filter)
- Storage: extend `shortcuts.json` with a `schedules` array

**Why it's compelling:** Solves muscle-memory context leakage during deep work or meetings. More granular than macOS Focus Modes, which can't mute individual hotkeys.

---

## 3. Global Command Palette
**Lens:** UX Design | **Effort:** Medium

A single global hotkey (e.g., `⌘⇧H`) summons a Spotlight-style floating overlay. Type to filter your shortcuts or run commands like "disable all," "quit." Dismiss with Escape. Keyboard-first; no menu bar dependency.

- New `NSPanel`-backed SwiftUI window scene with dark translucent overlay
- Filter/search logic reuses existing `AppSearchView` patterns
- Bind via `ShortcutManager` as a dedicated system-level shortcut
- Actions: activate app, toggle enable, open settings, quit

**Why it's compelling:** LSUIElement apps are invisible. A keyboard-summoned palette gives users a tactile, discoverable entry point — making the app feel present and alive rather than forgettable.

---

## 4. Usage Insights & Smart Suggestions
**Lens:** UX Design | **Effort:** Low-Medium

HotApp silently tracks per-shortcut invocation counts and time-of-day patterns. Settings shows "You've launched Figma 47× this week" and surfaces suggestions: "You frequently open Bear but have no hotkey — add one?" Also bubbles most-used apps to the top of `AppSearchView`.

- Lightweight `UsageLog: Codable` — append timestamp + bundleID on each activation
- Aggregate on Settings open (no runtime overhead)
- New "Insights" tab or inline badges in `ShortcutListView`
- Suggestion heuristics: apps opened 10+ times without a hotkey = suggest

**Why it's compelling:** Makes the app's compounding value visible. Guides new users toward optimal shortcut setup without a tutorial. Drives retention through personalization.

---

## 5. Focus Mode Context Switching
**Lens:** macOS Platform | **Effort:** Medium

Bind different shortcut sets to macOS Focus Modes. `⌘1` = Mail in Work mode; `⌘1` = Messages in Personal mode. HotApp monitors the active Focus Mode and calls `refreshRegistrations()` with the filtered shortcut set.

- Add `focusModeName: String?` to `AppShortcut` (nil = active in all modes)
- Monitor `UserDefaults` key `com.apple.controlcenter.lastSeenFocusIdentifier` via KVO
- On mode change, filter `store.shortcuts` and re-register with `ShortcutManager`
- UI: dropdown in shortcut row for mode assignment

**Why it's compelling:** Context-aware computing at the OS layer. Eliminates the mismatch between cognitive mode (work vs. personal) and available hotkeys. Zero new dependencies.

---

## 6. Spotlight & Siri Integration
**Lens:** macOS Platform | **Effort:** High

Index each shortcut as an `NSUserActivity` + `CSSearchableItem`. Users can type "Switch to Terminal" in Spotlight or say "Hey Siri, switch to Terminal." HotApp intercepts the activity and dispatches the toggle.

- Register `NSUserActivityTypes` in `Info.plist`
- On shortcut CRUD, update `CSSearchableIndex` with app name + combo as keywords
- Handle `application(_:continue:restorationHandler:)` → dispatch toggle
- Siri routing is automatic once `NSUserActivity` is indexed

**Why it's compelling:** Makes HotApp accessible to non-hotkey users (RSI, motor impairment, voice-first workflows). Integrates into Shortcuts.app automations. A rare macOS app with real Spotlight depth.

---

## 7. Predictive App Switching ("Crystal Ball")
**Lens:** AI/ML | **Effort:** Medium

On-device ML (CoreML/Create ML) learns temporal usage patterns — "9am → Terminal → Slack → Mail" — and predicts the next likely switch 5–30 seconds before the hotkey is pressed. Surfaces the prediction as a subtle menu bar badge or transient overlay. Ignored predictions self-correct the model.

- Feature vector: hour-of-day, day-of-week, last 3 apps activated, calendar event presence
- Lightweight sequence model (~1MB), trained locally on the usage log from Idea 4
- Inference on each activation event, <5ms on Apple Silicon
- UI: optional predictive badge on status item icon

**Why it's compelling:** Flips UX from reactive to anticipatory. After 2–4 weeks of use, the "right" app is already surfaced before the user reaches for a key. No cloud, no privacy cost.

---

## 8. Natural Language Intent Recognition
**Lens:** AI/ML | **Effort:** Medium-High

A text field (or voice via `Speech.framework`) in the menu lets users type or say intent: "write email" → Mail, "check builds" → Terminal. A lightweight on-device intent classifier (rule-based → MLX.Swift quantized model) maps intent to the most likely app, with a ranked disambiguation menu for ambiguous queries.

- Start with keyword/regex intent trees (zero ML dependency for v1)
- Graduate to quantized Mistral 3B / on-device embedding model
- Cache learned aliases: "check code" → VS Code, refined over time
- Accessibility mode: `Speech.SFSpeechRecognizer` for voice input

**Why it's compelling:** Removes the "what was the combo?" cognitive tax entirely. Hotkey-free switching for users who don't memorize bindings. Accessibility superpower for hands-free workflows.

---

## 9. Project Context Layers
**Lens:** Competitive | **Effort:** Medium

Users define named contexts ("Design," "Backend," "Writing"). Each shortcut can belong to one or more contexts. A dedicated hotkey (`⌘⇧~`) cycles contexts with a transient overlay. Only shortcuts assigned to the active context are registered at any time.

- Add `contexts: [String]` to `AppShortcut` (empty = global)
- `ContextManager` service tracks active context in `UserDefaults`
- On context switch, filter shortcuts and call `refreshRegistrations()`
- UI: "Manage Contexts" panel; multi-select chips per shortcut row

**Why it's compelling:** Eliminates decision fatigue across multi-project workflows. No competitor (Thor, Raycast, Alfred) understands project context. Designers don't want to see Postgres; backend devs don't want Figma.

---

## 10. Multi-App Activation Sequences
**Lens:** Competitive | **Effort:** High

A single hotkey activates multiple apps in sequence with per-step delay and launch mode (activate, background, hide-if-running). On repeat press, can cycle through different predefined sequences.

- New `AppSequence` model: `steps: [(shortcut: AppShortcut, delay: ms, mode: LaunchMode)]`
- `ShortcutManager` spawns the first action, then schedules subsequent via `DispatchSourceTimer`
- UI: sequence builder sheet with drag-to-reorder steps, delay slider, mode picker
- Conflict resolution: double-press cycles to next sequence or cancels in-flight

**Why it's compelling:** Addresses the "morning ritual" problem (launch 4 apps for work) with one keypress. No competitor is one-to-many. Behavioral lock-in: once users set up their ritual sequence, switching apps is unthinkable.

---

## 11. Hotkey Recipes (Shareable Profiles)
**Lens:** Extensibility | **Effort:** Medium

Export/import shortcut sets as `.hotrecipe` JSON files. Users share curated profiles ("Web Dev Stack," "Designer Essentials") via GitHub Gists, community registries, or direct file transfer. On import, fuzzy-match apps by bundle ID or name across machines.

- New `Recipe: Codable` struct: `{version, author, description, shortcuts[]}`
- `PersistenceService` gains `exportAsRecipe()` / `importRecipe(from: URL)` methods
- UI: share button in toolbar → save panel; import button → file picker + install sheet
- Bundle ID fuzzy matching for cross-machine portability (app name fallback)

**Why it's compelling:** Solves cold-start for new users. Community recipes make HotApp discoverable ("here's the Figma/Notion/Slack profile"). Mirrors VS Code snippet / Raycast extension flywheel.

---

## 12. Hotkey Hooks (Automation HTTP API)
**Lens:** Extensibility | **Effort:** High

A local HTTP server (`localhost:8377`) accepts JSON POSTs to dynamically register ephemeral shortcuts from any script, IDE plugin, or Shortcuts.app action. Shortcuts have optional TTLs and `onTrigger` webhook callbacks.

```bash
curl -X POST localhost:8377/hook \
  -d '{"keys":"cmd+shift+j","bundleId":"com.apple.Terminal","ttl":3600}'
```

- Minimal HTTP server via `NWListener` (no Vapor dependency)
- Shared secret auth via Keychain; rate limiting
- Ephemeral shortcuts stored in-memory + pruned on TTL expiry
- Wires into existing `ShortcutManager.register()` pipeline

**Why it's compelling:** Zero-config automation glue. CI/CD scripts, Alfred workflows, Raycast plugins, and IDE extensions can all register hotkeys programmatically. Enables a second ecosystem of open-source automation recipes built on top of HotApp.

---

## Priority Matrix

| # | Idea | Effort | Impact | Recommendation |
|---|------|--------|--------|----------------|
| 1 | Smart Workspaces | Medium | ★★★★★ | **Ship first** |
| 9 | Project Context Layers | Medium | ★★★★★ | Ship alongside #1 |
| 3 | Global Command Palette | Medium | ★★★★☆ | Early follow-up |
| 5 | Focus Mode Switching | Medium | ★★★★☆ | Early follow-up |
| 4 | Usage Insights | Low-Med | ★★★☆☆ | Low-hanging fruit |
| 11 | Hotkey Recipes | Medium | ★★★☆☆ | Community play |
| 2 | Scheduled Silencing | Med-High | ★★★☆☆ | Niche but loved |
| 7 | Predictive Switching | Medium | ★★★★☆ | Requires data (build after #4) |
| 10 | Activation Sequences | High | ★★★★☆ | Power-user milestone |
| 6 | Spotlight/Siri | High | ★★★☆☆ | Accessibility win |
| 8 | Natural Language | Med-High | ★★★☆☆ | Future |
| 12 | Hotkey Hooks API | High | ★★★☆☆ | Developer ecosystem play |

# Hotkey Recipes (Shareable Profiles)

**Date:** 2026-03-18
**Status:** Approved, not yet implemented
**Effort:** Medium

## Summary

Export/import shortcut sets as `.hotrecipe` JSON files. Users share curated profiles ("Web Dev Stack," "Designer Essentials") via GitHub Gists or direct file transfer. On import, resolve apps by bundle ID (with name-based fallback) and apply key combos automatically.

## Decisions

| Decision | Choice |
|----------|--------|
| Key combos in recipe? | Yes -- full mapping (app + key combo) |
| Conflict handling on import | Merge, skip duplicates (existing shortcuts untouched) |
| Export scope | Export all shortcuts at once |
| UI placement | Toolbar bottom bar in ShortcutListView |
| Architecture | Standalone Recipe struct (Approach A) -- clean transfer object, no model pollution |

## Data Model

### `Recipe` (new file: `Sources/HotApp/Models/Recipe.swift`)

```swift
struct Recipe: Codable, Sendable {
    let version: Int          // 1
    let entries: [RecipeEntry]
}

struct RecipeEntry: Codable, Sendable {
    let bundleIdentifier: String  // "com.apple.Safari"
    let displayName: String       // "Safari"
    let keyCode: Int              // Carbon key code from KeyboardShortcuts
    let carbonModifiers: Int      // Carbon modifier flags
}
```

Key points:
- **Carbon key code + modifiers** -- matches `KeyboardShortcuts.Shortcut` internal representation for exact round-tripping.
- **No `bundleURL`** -- URLs are machine-specific. Resolved on import.
- **No `author`/`description` metadata** -- YAGNI for v1. Users can encode context in the filename (e.g., "Web Dev Stack.hotrecipe"). Avoids needing a metadata input UI.
- **Version field** for future format evolution. On import, if `recipe.version > 1`, throw an error prompting the user to update HotApp. Prevents silent data loss from future format changes.
- **File extension**: `.hotrecipe` (JSON underneath).

## Export Flow

### `RecipeService` (new file: `Sources/HotApp/Services/RecipeService.swift`)

```swift
enum RecipeError: LocalizedError {
    case unsupportedVersion(Int)
    // errorDescription provides user-facing message
}

struct ImportResult {
    var added: Int = 0
    var skippedDuplicate: Int = 0
    var skippedNotFound: Int = 0
    var skippedKeyConflict: Int = 0
    var total: Int { added + skippedDuplicate + skippedNotFound + skippedKeyConflict }
}

struct RecipeService: Sendable {
    func exportRecipe(from shortcuts: [AppShortcut]) throws -> Data
    func importRecipe(from data: Data) throws -> Recipe
    // importRecipe throws RecipeError.unsupportedVersion if version > 1
}
```

**Export logic:**
1. Iterate `shortcuts` array.
2. For each, call `KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName)` to get the key combo.
3. Skip entries with no combo assigned.
4. Build `RecipeEntry` with `bundleIdentifier`, `displayName`, `carbonKeyCode`, `carbonModifiers`.
5. Wrap in `Recipe(version: 1, ...)` and JSON-encode with `.prettyPrinted + .sortedKeys`.

**UI trigger:** Export button in toolbar -> `NSSavePanel` with `.hotrecipe` extension -> write Data to selected URL.

## Import Flow

**Import logic (in `ShortcutStore`):**
1. UI: Import button in toolbar -> `NSOpenPanel` filtered to `.hotrecipe`.
2. Read file, decode via `RecipeService.importRecipe(from:)` -> `Recipe`.
3. **Version gate**: If `recipe.version > 1`, throw `RecipeError.unsupportedVersion` and show an alert prompting the user to update HotApp. Abort import.
4. For each `RecipeEntry`:
   - **Skip** if `bundleIdentifier` already exists in `store.shortcuts` (app duplicate).
   - **Resolve app location**:
     1. `NSWorkspace.shared.urlForApplication(withBundleIdentifier:)` -- exact match.
     2. `AppScanner` name-based scan -- fuzzy fallback by `displayName`.
     3. Skip + log warning if not found.
   - **Check key combo conflict**: Build the incoming `(keyCode, carbonModifiers)` and check if any existing shortcut -- or any earlier entry in this same import batch -- already uses that combo (via `KeyboardShortcuts.getShortcut` for existing, plus a local `Set` of combos assigned during this import). If conflict found, still add the app but skip writing the combo (leave recorder blank). Count as "key conflict" in summary.
   - Create `AppShortcut(bundleURL: resolvedURL, bundleIdentifier: entry.bundleIdentifier, displayName: entry.displayName)`.
   - Add to store via `store.add(shortcut)`.
   - Write key combo (if no conflict): `KeyboardShortcuts.setShortcut(.init(carbonKeyCode: entry.keyCode, carbonModifiers: entry.carbonModifiers), for: shortcut.keyboardShortcutName)`.
5. Post `.shortcutsDidChange` notification once after all entries processed.
6. Return `ImportResult` struct. Show summary alert built from the result: "Imported 5 of 7 shortcuts (1 app not found, 1 key conflict)". Clean, testable, no string assembly buried in import logic.

## UI Changes

### ShortcutListView toolbar layout

```
[ + ] [ - ]                    [ Import ] [ Export ]
```

- **Left side**: existing `+` and `-` buttons (unchanged).
- **Right side**: two new icon buttons after `Spacer()`.
  - **Import**: `square.and.arrow.down` SF Symbol -> triggers `NSOpenPanel`.
  - **Export**: `square.and.arrow.up` SF Symbol -> triggers `NSSavePanel`. Disabled when `store.shortcuts.isEmpty`.
- **Import result feedback**: `.alert()` modifier showing "Imported X of Y shortcuts (Z apps not found)".

**Drag-and-drop import**: Extend the existing `.onDrop(of: [.fileURL])` handler in `ShortcutListView` to also accept `.hotrecipe` files. If the dropped file has a `.hotrecipe` extension, trigger the import flow instead of the add-app flow. Natural UX that piggybacks on existing infrastructure.

No new views or sheets required -- standard macOS file panels only.

## Files Changed/Created

| File | Change |
|------|--------|
| `Sources/HotApp/Models/Recipe.swift` | **New** -- `Recipe` + `RecipeEntry` structs |
| `Sources/HotApp/Services/RecipeService.swift` | **New** -- export/import serialization logic |
| `Sources/HotApp/Views/ShortcutListView.swift` | **Modified** -- add import/export toolbar buttons, file panel triggers, import result alert |
| `Sources/HotApp/Models/ShortcutStore.swift` | **Modified** -- add `importRecipe(_:)` method (merge + app resolution + key combo writing) |

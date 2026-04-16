import Foundation
import KeyboardShortcuts

struct RecipeService: Sendable {
    func exportRecipe(from shortcuts: [AppShortcut]) throws -> Data {
        var entries: [RecipeEntry] = []
        for shortcut in shortcuts {
            guard let combo = KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName) else {
                continue
            }
            entries.append(RecipeEntry(
                bundleIdentifier: shortcut.bundleIdentifier,
                displayName: shortcut.displayName,
                keyCode: combo.carbonKeyCode,
                carbonModifiers: combo.carbonModifiers
            ))
        }

        let recipe = Recipe(version: Recipe.currentVersion, entries: entries)
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        return try encoder.encode(recipe)
    }

    func importRecipe(from data: Data) throws -> Recipe {
        let recipe = try JSONDecoder().decode(Recipe.self, from: data)
        if recipe.version > Recipe.currentVersion {
            throw RecipeError.unsupportedVersion(recipe.version)
        }
        return recipe
    }
}

import AppKit
import Foundation
import KeyboardShortcuts
import Observation

@Observable @MainActor
final class ShortcutStore {
    var shortcuts: [AppShortcut] = []
    var globalEnabled: Bool {
        didSet { UserDefaults.standard.set(globalEnabled, forKey: "globalEnabled") }
    }

    weak var shortcutManager: ShortcutManager?
    private let persistence: any PersistenceService
    private var saveTask: Task<Void, Never>?

    init(persistence: any PersistenceService = JSONFilePersistence()) {
        self.globalEnabled = UserDefaults.standard.object(forKey: "globalEnabled") as? Bool ?? true
        self.persistence = persistence
        load()
    }

    func add(_ shortcut: AppShortcut) {
        guard !shortcuts.contains(where: { $0.bundleIdentifier == shortcut.bundleIdentifier }) else {
            return
        }
        shortcuts.append(shortcut)
        saveImmediately()
        if globalEnabled, shortcut.isEnabled {
            shortcutManager?.register(shortcut)
        }
    }

    func remove(id: UUID) {
        guard let shortcut = shortcuts.first(where: { $0.id == id }) else { return }
        shortcutManager?.unregister(shortcut)
        KeyboardShortcuts.reset(shortcut.keyboardShortcutName)
        shortcuts.removeAll { $0.id == id }
        saveImmediately()
    }

    func move(from source: IndexSet, to destination: Int) {
        shortcuts.move(fromOffsets: source, toOffset: destination)
        save()
    }

    func update(_ shortcut: AppShortcut) {
        guard let index = shortcuts.firstIndex(where: { $0.id == shortcut.id }) else { return }
        shortcuts[index] = shortcut
        save()
    }

    func toggleEnabled(id: UUID) {
        guard let index = shortcuts.firstIndex(where: { $0.id == id }) else { return }
        shortcuts[index].isEnabled.toggle()
        let shortcut = shortcuts[index]
        if globalEnabled {
            if shortcut.isEnabled {
                shortcutManager?.register(shortcut)
            } else {
                shortcutManager?.unregister(shortcut)
            }
        }
        save()
    }

    func refreshStaleness() {
        for i in shortcuts.indices {
            shortcuts[i].isStale = !FileManager.default.fileExists(
                atPath: shortcuts[i].bundleURL.path)
        }
    }

    func refreshStaleness(for bundleIdentifier: String) {
        for i in shortcuts.indices where shortcuts[i].bundleIdentifier == bundleIdentifier {
            shortcuts[i].isStale = !FileManager.default.fileExists(
                atPath: shortcuts[i].bundleURL.path)
        }
    }

    func save() {
        saveTask?.cancel()
        saveTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            self?.persistToDisk()
        }
    }

    func saveImmediately() {
        saveTask?.cancel()
        persistToDisk()
    }

    private func persistToDisk() {
        do {
            try persistence.save(shortcuts)
        } catch {
            print("Failed to save shortcuts: \(error)")
        }
    }

    func load() {
        do {
            shortcuts = try persistence.load()
        } catch {
            shortcuts = []
        }
        refreshStaleness()
        cleanupOrphanedDefaults()
    }

    var isFirstLaunch: Bool {
        !FileManager.default.fileExists(
            atPath: JSONFilePersistence.defaultFileURL.path)
    }

    func importRecipe(_ recipe: Recipe) -> ImportResult {
        var result = ImportResult()
        // Track combos assigned in this batch to detect intra-recipe duplicates
        var batchCombos: Set<String> = []

        // Collect existing combos for conflict detection
        var existingCombos: Set<String> = []
        for shortcut in shortcuts {
            if let combo = KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName) {
                existingCombos.insert("\(combo.carbonKeyCode)_\(combo.carbonModifiers)")
            }
        }

        for entry in recipe.entries {
            // Skip if app already has a shortcut
            if shortcuts.contains(where: { $0.bundleIdentifier == entry.bundleIdentifier }) {
                result.skippedDuplicate += 1
                continue
            }

            // Resolve app location
            guard let bundleURL = resolveAppURL(bundleIdentifier: entry.bundleIdentifier, displayName: entry.displayName) else {
                result.skippedNotFound += 1
                continue
            }

            let shortcut = AppShortcut(
                bundleURL: bundleURL,
                bundleIdentifier: entry.bundleIdentifier,
                displayName: entry.displayName
            )

            // Check key combo conflict (existing + intra-batch)
            let comboKey = "\(entry.keyCode)_\(entry.carbonModifiers)"
            let hasConflict = existingCombos.contains(comboKey) || batchCombos.contains(comboKey)

            shortcuts.append(shortcut)

            if hasConflict {
                result.skippedKeyConflict += 1
            } else {
                let combo = KeyboardShortcuts.Shortcut(
                    carbonKeyCode: entry.keyCode,
                    carbonModifiers: entry.carbonModifiers
                )
                KeyboardShortcuts.setShortcut(combo, for: shortcut.keyboardShortcutName)
                batchCombos.insert(comboKey)
            }

            result.added += 1
        }

        if result.added > 0 {
            saveImmediately()
        }

        return result
    }

    private func resolveAppURL(bundleIdentifier: String, displayName: String) -> URL? {
        // 1. Exact match by bundle ID
        if let url = NSWorkspace.shared.urlForApplication(withBundleIdentifier: bundleIdentifier) {
            return url
        }

        // 2. Fuzzy fallback: scan known directories for matching display name
        let fm = FileManager.default
        for directory in AppScanner.searchDirectories {
            guard let contents = try? fm.contentsOfDirectory(
                at: directory,
                includingPropertiesForKeys: nil,
                options: [.skipsHiddenFiles]
            ) else { continue }

            for url in contents where url.pathExtension == "app" {
                let name = url.deletingPathExtension().lastPathComponent
                if name.localizedCaseInsensitiveCompare(displayName) == .orderedSame {
                    return url
                }
            }
        }

        return nil
    }

    func cleanupOrphanedDefaults() {
        let validNames = Set(shortcuts.map(\.shortcutName))
        let defaults = UserDefaults.standard
        let allKeys = defaults.dictionaryRepresentation().keys
        for key in allKeys where key.hasPrefix("KeyboardShortcuts_") {
            let name = String(key.dropFirst("KeyboardShortcuts_".count))
            if name.hasPrefix("hotapp_"), !validNames.contains(name) {
                defaults.removeObject(forKey: key)
            }
        }
    }
}

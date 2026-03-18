import Foundation
import KeyboardShortcuts
import Observation

@Observable @MainActor
final class ShortcutStore {
    var shortcuts: [AppShortcut] = []
    var globalEnabled: Bool {
        didSet { UserDefaults.standard.set(globalEnabled, forKey: "globalEnabled") }
    }

    private let persistence: any PersistenceService

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
        save()
    }

    func remove(id: UUID) {
        guard let shortcut = shortcuts.first(where: { $0.id == id }) else { return }
        KeyboardShortcuts.reset(shortcut.keyboardShortcutName)
        shortcuts.removeAll { $0.id == id }
        save()
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
        save()
    }

    func refreshStaleness() {
        for i in shortcuts.indices {
            shortcuts[i].isStale = !FileManager.default.fileExists(
                atPath: shortcuts[i].bundleURL.path)
        }
    }

    func save() {
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
        cleanupOrphanedDefaults()
    }

    var isFirstLaunch: Bool {
        !FileManager.default.fileExists(
            atPath: JSONFilePersistence.defaultFileURL.path)
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

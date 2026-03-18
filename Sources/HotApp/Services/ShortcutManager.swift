import AppKit
import KeyboardShortcuts

@MainActor
final class ShortcutManager {
    private let store: ShortcutStore

    init(store: ShortcutStore) {
        self.store = store
    }

    func registerAll() {
        for shortcut in store.shortcuts where shortcut.isEnabled {
            register(shortcut)
        }
    }

    func unregisterAll() {
        for shortcut in store.shortcuts {
            KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
        }
    }

    func register(_ shortcut: AppShortcut) {
        let name = shortcut.keyboardShortcutName
        let bundleIdentifier = shortcut.bundleIdentifier
        let bundleURL = shortcut.bundleURL

        KeyboardShortcuts.onKeyUp(for: name) { [weak self] in
            guard let self else { return }
            self.toggleApp(bundleIdentifier: bundleIdentifier, bundleURL: bundleURL)
        }
    }

    func unregister(_ shortcut: AppShortcut) {
        KeyboardShortcuts.disable(shortcut.keyboardShortcutName)
    }

    func refreshRegistrations() {
        unregisterAll()
        guard store.globalEnabled else { return }
        registerAll()
    }

    private func toggleApp(bundleIdentifier: String, bundleURL: URL) {
        guard store.globalEnabled else { return }

        let workspace = NSWorkspace.shared
        if let frontmost = workspace.frontmostApplication,
           frontmost.bundleIdentifier == bundleIdentifier
        {
            frontmost.hide()
        } else {
            let config = NSWorkspace.OpenConfiguration()
            config.activates = true
            workspace.openApplication(at: bundleURL, configuration: config)
        }
    }
}

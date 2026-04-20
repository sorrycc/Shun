import AppKit

@MainActor
final class StatusItemController: NSObject, NSMenuDelegate {
    private let statusItem: NSStatusItem
    private let store: ShortcutStore
    private let onOpenSettings: () -> Void

    init(store: ShortcutStore, onOpenSettings: @escaping () -> Void) {
        self.store = store
        self.onOpenSettings = onOpenSettings
        self.statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        super.init()

        if let button = statusItem.button {
            button.image = Self.loadMenuBarIcon()
        }

        let menu = NSMenu()
        menu.delegate = self
        statusItem.menu = menu
    }

    // MARK: - NSMenuDelegate

    func menuNeedsUpdate(_ menu: NSMenu) {
        rebuildMenu(menu)
    }

    private func rebuildMenu(_ menu: NSMenu) {
        menu.removeAllItems()

        let version =
            Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String
            ?? "1.0"
        let titleItem = NSMenuItem(title: "Shun v\(version)", action: nil, keyEquivalent: "")
        titleItem.isEnabled = false
        menu.addItem(titleItem)

        menu.addItem(.separator())

        let enabledItem = NSMenuItem(
            title: "Enabled",
            action: #selector(toggleGlobalEnabled),
            keyEquivalent: ""
        )
        enabledItem.target = self
        enabledItem.state = store.globalEnabled ? .on : .off
        menu.addItem(enabledItem)

        let settingsItem = NSMenuItem(
            title: "Settings...",
            action: #selector(openSettings),
            keyEquivalent: ","
        )
        settingsItem.target = self
        menu.addItem(settingsItem)

        let checkUpdatesItem = NSMenuItem(
            title: "Check for Updates…",
            action: #selector(checkForUpdates),
            keyEquivalent: ""
        )
        checkUpdatesItem.target = self
        menu.addItem(checkUpdatesItem)

        menu.addItem(.separator())

        let quitItem = NSMenuItem(
            title: "Quit Shun",
            action: #selector(NSApplication.terminate(_:)),
            keyEquivalent: "q"
        )
        menu.addItem(quitItem)
    }

    @objc private func toggleGlobalEnabled() {
        store.globalEnabled.toggle()
        NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
    }

    @objc private func openSettings() {
        onOpenSettings()
    }

    @objc private func checkForUpdates() {
        NotificationCenter.default.post(name: .checkForUpdates, object: nil)
    }

    private static func loadMenuBarIcon() -> NSImage {
        let image = NSImage(named: "MenuBarIcon") ?? NSImage()
        image.isTemplate = true
        image.accessibilityDescription = "Shun"
        return image
    }
}

extension Notification.Name {
    static let shortcutsDidChange = Notification.Name("shortcutsDidChange")
    static let openSettings = Notification.Name("openSettings")
    static let checkForUpdates = Notification.Name("checkForUpdates")
}

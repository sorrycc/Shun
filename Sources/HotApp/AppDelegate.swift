import AppKit

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let store = ShortcutStore()
    lazy var shortcutManager = ShortcutManager(store: store)
    private var statusItemController: StatusItemController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        statusItemController = StatusItemController(store: store) { [weak self] in
            self?.openSettings()
        }

        shortcutManager.refreshRegistrations()
        observeWorkspaceNotifications()
        observeShortcutChanges()

        if store.isFirstLaunch {
            Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(500))
                openSettings()
            }
        }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        openSettings()
        return false
    }

    private func openSettings() {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .openSettings, object: nil)
    }

    private func observeWorkspaceNotifications() {
        let center = NSWorkspace.shared.notificationCenter

        center.addObserver(
            forName: NSWorkspace.didLaunchApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                self?.store.refreshStaleness()
            }
        }

        center.addObserver(
            forName: NSWorkspace.didTerminateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                self?.store.refreshStaleness()
            }
        }
    }

    private func observeShortcutChanges() {
        NotificationCenter.default.addObserver(
            forName: .shortcutsDidChange,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                self?.shortcutManager.refreshRegistrations()
            }
        }
    }
}

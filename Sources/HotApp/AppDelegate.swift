import AppKit
import ApplicationServices

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let store = ShortcutStore()
    lazy var shortcutManager = ShortcutManager(store: store)
    private var statusItemController: StatusItemController?

    func applicationDidFinishLaunching(_ notification: Notification) {
        store.shortcutManager = shortcutManager

        statusItemController = StatusItemController(store: store) { [weak self] in
            self?.openSettings()
        }

        checkAccessibilityPermission()
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
        ) { [weak self] notification in
            let bundleId = (notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication)?.bundleIdentifier
            Task { @MainActor in
                if let bundleId {
                    self?.store.refreshStaleness(for: bundleId)
                } else {
                    self?.store.refreshStaleness()
                }
            }
        }

        center.addObserver(
            forName: NSWorkspace.didTerminateApplicationNotification,
            object: nil,
            queue: .main
        ) { [weak self] notification in
            let bundleId = (notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication)?.bundleIdentifier
            Task { @MainActor in
                if let bundleId {
                    self?.store.refreshStaleness(for: bundleId)
                } else {
                    self?.store.refreshStaleness()
                }
            }
        }
    }

    private func checkAccessibilityPermission() {
        let trusted = AXIsProcessTrustedWithOptions(
            ["AXTrustedCheckOptionPrompt": true] as CFDictionary
        )
        if !trusted {
            let alert = NSAlert()
            alert.messageText = "Accessibility Permission Required"
            alert.informativeText = "HotApp needs Accessibility access to register global hotkeys. Please grant access in System Settings → Privacy & Security → Accessibility, then relaunch."
            alert.alertStyle = .warning
            alert.addButton(withTitle: "Open System Settings")
            alert.addButton(withTitle: "Later")
            if alert.runModal() == .alertFirstButtonReturn {
                NSWorkspace.shared.open(
                    URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")!
                )
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

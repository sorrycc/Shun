import AppKit
import ApplicationServices

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let store = ShortcutStore()
    let usageStore = UsageStore()
    lazy var shortcutManager = ShortcutManager(store: store, usageStore: usageStore)
    private var statusItemController: StatusItemController?
    private var updater: UpdaterService?
    private var crashReporter: CrashReporter?
    private var telemetry: TelemetryService?

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Redundant in packaged builds (LSUIElement=true already sets this),
        // but required in `swift run` dev builds where no Info.plist exists.
        NSApp.setActivationPolicy(.accessory)

        store.shortcutManager = shortcutManager

        // Only start Sparkle when packaged — dev builds (swift run) have no SUFeedURL
        // in Info.plist and Sparkle would assert on init.
        if Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil {
            updater = UpdaterService()
        }

        statusItemController = StatusItemController(store: store) { [weak self] in
            self?.openSettings()
        }

        checkAccessibilityPermission()
        shortcutManager.refreshRegistrations()
        observeWorkspaceNotifications()
        observeShortcutChanges()
        observeUpdateChecks()

        if store.isFirstLaunch {
            Task { @MainActor in
                try? await Task.sleep(for: .milliseconds(500))
                openSettings()
            }
        }

        // Telemetry + crash upload are opt-out; gated on Info.plist like Sparkle.
        if let config = TelemetryConfig.fromBundle() {
            let crashReporter = CrashReporter(config: config)
            let telemetry = TelemetryService(config: config)
            self.crashReporter = crashReporter
            self.telemetry = telemetry
            Task {
                await crashReporter.start()
                await telemetry.start()
            }
        }
    }

    func applicationWillTerminate(_ notification: Notification) {
        usageStore.saveImmediately()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        openSettings()
        return false
    }

    private func openSettings() {
        NSApp.activate()
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
            ["AXTrustedCheckOptionPrompt": false] as CFDictionary
        )
        if !trusted {
            let alert = NSAlert()
            alert.messageText = "Accessibility Permission Required"
            alert.informativeText = "Shun needs Accessibility access to register global hotkeys. Please grant access in System Settings → Privacy & Security → Accessibility, then relaunch."
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

    private func observeUpdateChecks() {
        NotificationCenter.default.addObserver(
            forName: .checkForUpdates,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor in
                NSApp.activate()
                self?.updater?.checkForUpdates()
            }
        }
    }
}

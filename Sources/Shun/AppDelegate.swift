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
        observeWindowClose()

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
        // Activation + window fronting is handled by the app-level `.openSettings`
        // observer in ShunApp.init (which flips to `.regular` first — required on
        // Tahoe). All entry points (menu, first-launch, reopen) funnel through here.
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
                // Tahoe: flip to `.regular` so the Sparkle dialog can become
                // frontmost. observeWindowClose() reverts to `.accessory` once it
                // closes.
                NSApp.setActivationPolicy(.regular)
                NSApp.activate()
                self?.updater?.checkForUpdates()
            }
        }
    }

    /// Reverts the activation policy to `.accessory` once no user-facing window
    /// remains. Registered once (here, on launch), so repeated Settings open/close
    /// never accumulates observers. Keyed on end state — not the closing window's
    /// identity — so incidental closes (menus, or the Settings window closing while
    /// a Sparkle window is still up) don't drop the Dock icon out from under a
    /// visible window. `canBecomeMain` admits the Settings + Sparkle windows but
    /// excludes the status-bar window.
    private func observeWindowClose() {
        NotificationCenter.default.addObserver(
            forName: NSWindow.willCloseNotification,
            object: nil,
            queue: .main
        ) { _ in
            // Defer one main-actor turn: willClose fires before the window leaves
            // the screen, so the predicate must run after it has settled.
            Task { @MainActor in
                let hasUserFacingWindow = NSApp.windows.contains {
                    $0.isVisible && $0.canBecomeMain
                }
                if !hasUserFacingWindow {
                    NSApp.setActivationPolicy(.accessory)
                }
            }
        }
    }
}

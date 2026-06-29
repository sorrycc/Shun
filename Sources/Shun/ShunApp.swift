import AppKit
import SwiftUI

@main
struct ShunApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    init() {
        // App-level observer so it survives the Settings window's teardown. This
        // used to be an `.onReceive` on SettingsView, but a `Window` scene tears
        // its content (and the subscriber) down on close, so the next menu click
        // posted `.openSettings` into the void and nothing reopened the window.
        // Registered with `queue: .main`, so the block is delivered on the main
        // thread — that delivery guarantee (not the poster's thread) is what makes
        // `assumeIsolated` sound here. The closure captures no instance state.
        NotificationCenter.default.addObserver(
            forName: .openSettings,
            object: nil,
            queue: .main
        ) { _ in
            MainActor.assumeIsolated {
                // macOS Tahoe tightened activation for `.accessory` apps — flip to
                // `.regular` so the app can become frontmost and the window can take
                // key status. AppDelegate's shared willClose observer reverts to
                // `.accessory` once no user-facing window remains.
                NSApp.setActivationPolicy(.regular)
                NSApp.activate(ignoringOtherApps: true)
                if let window = NSApp.windows.first(where: { $0.title == "Shun Settings" }) {
                    window.makeKeyAndOrderFront(nil)
                    window.orderFrontRegardless()
                }
            }
        }
    }

    var body: some Scene {
        Window("Shun Settings", id: "settings") {
            SettingsView()
                .environment(appDelegate.store)
                .environment(appDelegate.usageStore)
        }
        .defaultSize(width: 520, height: 400)
        .windowResizability(.contentSize)
    }
}

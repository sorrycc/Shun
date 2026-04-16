import SwiftUI

@main
struct ShunApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @Environment(\.openWindow) private var openWindow

    var body: some Scene {
        Window("Shun Settings", id: "settings") {
            SettingsView()
                .environment(appDelegate.store)
                .environment(appDelegate.usageStore)
                .onReceive(NotificationCenter.default.publisher(for: .openSettings)) { _ in
                    openWindow(id: "settings")
                }
        }
        .defaultSize(width: 520, height: 400)
        .windowResizability(.contentSize)
    }
}

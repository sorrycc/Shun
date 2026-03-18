import ServiceManagement
import SwiftUI

struct SettingsView: View {
    private enum Tab: String, CaseIterable {
        case shortcuts = "Shortcuts"
        case general = "General"
    }

    @State private var selectedTab: Tab = .shortcuts

    var body: some View {
        TabView(selection: $selectedTab) {
            ShortcutListView()
                .tabItem { Label("Shortcuts", systemImage: "keyboard") }
                .tag(Tab.shortcuts)

            GeneralSettingsView()
                .tabItem { Label("General", systemImage: "gearshape") }
                .tag(Tab.general)
        }
        .frame(minWidth: 520, minHeight: 400)
    }
}

struct GeneralSettingsView: View {
    @Environment(ShortcutStore.self) private var store
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled

    var body: some View {
        Form {
            Toggle("Launch at Login", isOn: $launchAtLogin)
                .onChange(of: launchAtLogin) { _, newValue in
                    do {
                        if newValue {
                            try SMAppService.mainApp.register()
                        } else {
                            try SMAppService.mainApp.unregister()
                        }
                    } catch {
                        launchAtLogin = !newValue
                        print("Failed to update login item: \(error)")
                    }
                }

            @Bindable var store = store
            Toggle("Enable All Shortcuts", isOn: $store.globalEnabled)
                .onChange(of: store.globalEnabled) { _, _ in
                    NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
                }
        }
        .formStyle(.grouped)
        .padding()
    }
}

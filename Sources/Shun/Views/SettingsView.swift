import ServiceManagement
import SwiftUI

struct SettingsView: View {
    private enum Tab: String, CaseIterable {
        case shortcuts = "Shortcuts"
        case general = "General"
        case insights = "Insights"
        case privacy = "Privacy"
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

            InsightsView()
                .tabItem { Label("Insights", systemImage: "chart.bar") }
                .tag(Tab.insights)

            PrivacySettingsView()
                .tabItem { Label("Privacy", systemImage: "hand.raised") }
                .tag(Tab.privacy)
        }
        .frame(minWidth: 520, minHeight: 400)
        .onDisappear {
            AppScanner.shared.invalidateCache()
        }
    }
}

struct GeneralSettingsView: View {
    @Environment(ShortcutStore.self) private var store
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled
    // Sparkle persists this key in UserDefaults; bind directly so the UI mirrors Sparkle's state.
    @AppStorage("SUEnableAutomaticChecks") private var automaticallyCheckForUpdates = true

    private var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String
            ?? "unknown"
    }

    private var updatesConfigured: Bool {
        Bundle.main.object(forInfoDictionaryKey: "SUFeedURL") != nil
    }

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

            if updatesConfigured {
                Section {
                    HStack {
                        Text("Current version")
                        Spacer()
                        Text(appVersion).foregroundStyle(.secondary)
                    }
                    Toggle(
                        "Automatically check for updates",
                        isOn: $automaticallyCheckForUpdates
                    )
                    HStack {
                        Spacer()
                        Button("Check for Updates…") {
                            NotificationCenter.default.post(
                                name: .checkForUpdates,
                                object: nil
                            )
                        }
                    }
                } header: {
                    Text("Updates")
                }
            }
        }
        .formStyle(.grouped)
        .padding()
    }
}

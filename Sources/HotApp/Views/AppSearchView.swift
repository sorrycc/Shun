import SwiftUI

struct AppSearchView: View {
    @Environment(ShortcutStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @State private var searchText = ""
    @State private var apps: [ScannedApp] = []
    @State private var isLoading = true

    private let scanner = AppScanner()

    var filteredApps: [ScannedApp] {
        let allApps = apps
        if searchText.isEmpty {
            return allApps
        }
        return allApps.filter { $0.name.localizedCaseInsensitiveContains(searchText) }
    }

    var body: some View {
        VStack(spacing: 0) {
            searchField
            Divider()
            if isLoading {
                loadingView
            } else {
                appList
            }
        }
        .frame(width: 400, height: 500)
        .task {
            apps = scanner.scan()
            isLoading = false
        }
    }

    private var searchField: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Search applications...", text: $searchText)
                .textFieldStyle(.plain)
                .onSubmit {
                    let existingIds = Set(store.shortcuts.map(\.bundleIdentifier))
                    if let first = filteredApps.first(where: { !existingIds.contains($0.bundleIdentifier) }) {
                        addApp(first)
                    }
                }
        }
        .padding(12)
    }

    private var loadingView: some View {
        VStack {
            ProgressView()
                .controlSize(.large)
            Text("Scanning applications...")
                .foregroundStyle(.secondary)
                .padding(.top, 8)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var appList: some View {
        let existingIds = Set(store.shortcuts.map(\.bundleIdentifier))

        return List(filteredApps) { app in
            let alreadyAdded = existingIds.contains(app.bundleIdentifier)

            Button(action: { if !alreadyAdded { addApp(app) } }) {
                HStack(spacing: 12) {
                    Image(nsImage: app.icon)
                        .resizable()
                        .frame(width: 32, height: 32)

                    VStack(alignment: .leading) {
                        Text(app.name)
                            .lineLimit(1)
                        Text(app.bundleIdentifier)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }

                    Spacer()

                    if alreadyAdded {
                        Text("Added")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 2)
                            .background(.secondary.opacity(0.15), in: Capsule())
                    }
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .opacity(alreadyAdded ? 0.5 : 1.0)
        }
        .listStyle(.plain)
    }

    private func addApp(_ app: ScannedApp) {
        let shortcut = AppShortcut(
            bundleURL: app.bundleURL,
            bundleIdentifier: app.bundleIdentifier,
            displayName: app.name
        )
        store.add(shortcut)
        NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
        dismiss()
    }
}

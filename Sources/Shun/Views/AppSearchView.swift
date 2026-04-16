import SwiftUI

struct AppSearchView: View {
    @Environment(ShortcutStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    @State private var searchText = ""
    @State private var apps: [ScannedApp] = []
    @State private var isLoading = true
    @State private var selectedIndex = 0

    var filteredApps: [ScannedApp] {
        if searchText.isEmpty {
            return apps
        }
        return apps.filter { $0.name.localizedCaseInsensitiveContains(searchText) }
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
            apps = await AppScanner.shared.scanAsync()
            isLoading = false
        }
        .onChange(of: searchText) { _, _ in
            selectedIndex = 0
        }
        .onKeyPress(.upArrow) {
            if selectedIndex > 0 {
                selectedIndex -= 1
            }
            return .handled
        }
        .onKeyPress(.downArrow) {
            if selectedIndex < filteredApps.count - 1 {
                selectedIndex += 1
            }
            return .handled
        }
    }

    private var searchField: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Search applications...", text: $searchText)
                .textFieldStyle(.plain)
                .onSubmit {
                    addSelectedApp()
                }
            Button("Done") {
                dismiss()
            }
            .buttonStyle(.bordered)
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

        return ScrollViewReader { proxy in
            List {
                ForEach(Array(filteredApps.enumerated()), id: \.element.id) { index, app in
                    let alreadyAdded = existingIds.contains(app.bundleIdentifier)
                    let isSelected = index == selectedIndex

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
                            Image(systemName: "checkmark.circle.fill")
                                .foregroundStyle(.green)
                        }
                    }
                    .contentShape(Rectangle())
                    .onTapGesture {
                        selectedIndex = index
                        if !alreadyAdded { addApp(app) }
                    }
                    .listRowBackground(
                        isSelected
                            ? Color.accentColor.opacity(0.2)
                            : Color.clear
                    )
                    .id(app.id)
                    .opacity(alreadyAdded ? 0.5 : 1.0)
                }
            }
            .listStyle(.plain)
            .onChange(of: selectedIndex) { _, newValue in
                if newValue < filteredApps.count {
                    withAnimation {
                        proxy.scrollTo(filteredApps[newValue].id, anchor: nil)
                    }
                }
            }
        }
    }

    private func addSelectedApp() {
        let existingIds = Set(store.shortcuts.map(\.bundleIdentifier))
        guard selectedIndex < filteredApps.count else { return }
        let app = filteredApps[selectedIndex]
        guard !existingIds.contains(app.bundleIdentifier) else { return }
        addApp(app)
    }

    private func addApp(_ app: ScannedApp) {
        let shortcut = AppShortcut(
            bundleURL: app.bundleURL,
            bundleIdentifier: app.bundleIdentifier,
            displayName: app.name
        )
        store.add(shortcut)
        NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
    }
}

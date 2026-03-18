import KeyboardShortcuts
import SwiftUI
import UniformTypeIdentifiers

struct ShortcutListView: View {
    @Environment(ShortcutStore.self) private var store
    @State private var selection: UUID?
    @State private var showingAppSearch = false
    @State private var pendingDeleteIds: [UUID] = []
    @State private var showDeleteConfirmation = false

    var body: some View {
        VStack(spacing: 0) {
            if store.shortcuts.isEmpty {
                emptyState
            } else {
                shortcutList
            }
            toolbar
        }
        .sheet(isPresented: $showingAppSearch) {
            AppSearchView()
        }
        .onDrop(of: [.fileURL], isTargeted: nil) { providers in
            handleDrop(providers)
        }
        .confirmationDialog(
            deleteConfirmationTitle,
            isPresented: $showDeleteConfirmation
        ) {
            Button("Remove", role: .destructive) {
                for id in pendingDeleteIds {
                    store.remove(id: id)
                }
                if pendingDeleteIds.contains(where: { $0 == selection }) {
                    selection = nil
                }
                pendingDeleteIds = []
                NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
            }
        }
        .onAppear {
            store.refreshStaleness()
        }
    }

    private var deleteConfirmationTitle: String {
        if pendingDeleteIds.count == 1,
           let shortcut = store.shortcuts.first(where: { $0.id == pendingDeleteIds.first })
        {
            return "Remove shortcut for \(shortcut.displayName)?"
        }
        return "Remove \(pendingDeleteIds.count) shortcuts?"
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "keyboard")
                .font(.system(size: 48))
                .foregroundStyle(.secondary)
            Text("No shortcuts configured")
                .font(.headline)
                .foregroundStyle(.secondary)
            Text("Click + or drag apps here to get started")
                .font(.subheadline)
                .foregroundStyle(.tertiary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var shortcutList: some View {
        List(selection: $selection) {
            ForEach(store.shortcuts) { shortcut in
                ShortcutRow(shortcut: shortcut)
                    .tag(shortcut.id)
            }
            .onMove { source, destination in
                store.move(from: source, to: destination)
                NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
            }
            .onDelete { indexSet in
                pendingDeleteIds = indexSet.map { store.shortcuts[$0].id }
                showDeleteConfirmation = true
            }
        }
        .listStyle(.inset(alternatesRowBackgrounds: true))
    }

    private var toolbar: some View {
        HStack {
            Button(action: { showingAppSearch = true }) {
                Image(systemName: "plus")
            }

            Button(action: confirmRemoveSelected) {
                Image(systemName: "minus")
            }
            .disabled(selection == nil)

            Spacer()
        }
        .padding(8)
        .background(.bar)
    }

    private func confirmRemoveSelected() {
        guard let id = selection else { return }
        pendingDeleteIds = [id]
        showDeleteConfirmation = true
    }

    private func handleDrop(_ providers: [NSItemProvider]) -> Bool {
        var handled = false
        for provider in providers {
            provider.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) {
                data, _ in
                guard let data = data as? Data,
                    let url = URL(dataRepresentation: data, relativeTo: nil),
                    url.pathExtension == "app"
                else { return }

                Task { @MainActor in
                    addApp(from: url)
                }
            }
            handled = true
        }
        return handled
    }

    @MainActor
    private func addApp(from url: URL) {
        guard let bundle = Bundle(url: url),
            let identifier = bundle.bundleIdentifier
        else { return }

        let name =
            bundle.object(forInfoDictionaryKey: "CFBundleName") as? String
            ?? bundle.object(forInfoDictionaryKey: "CFBundleDisplayName") as? String
            ?? url.deletingPathExtension().lastPathComponent

        let shortcut = AppShortcut(
            bundleURL: url,
            bundleIdentifier: identifier,
            displayName: name
        )
        store.add(shortcut)
        NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
    }
}

struct ShortcutRow: View {
    let shortcut: AppShortcut
    @Environment(ShortcutStore.self) private var store
    @State private var conflictNotice: String?

    var body: some View {
        HStack(spacing: 12) {
            appIcon
            appName
            Spacer()
            VStack(alignment: .trailing, spacing: 2) {
                recorder
                if let conflictNotice {
                    Text(conflictNotice)
                        .font(.caption2)
                        .foregroundStyle(.orange)
                        .transition(.opacity)
                }
            }
            enableToggle
        }
        .padding(.vertical, 4)
        .opacity(shortcut.isStale ? 0.5 : 1.0)
        .overlay(alignment: .topTrailing) {
            if shortcut.isStale {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(.yellow)
                    .font(.caption)
                    .help("App not found at \(shortcut.bundleURL.path)")
            }
        }
    }

    private var appIcon: some View {
        Image(nsImage: AppScanner.shared.cachedIcon(for: shortcut.bundleURL))
            .resizable()
            .frame(width: 32, height: 32)
    }

    private var appName: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(shortcut.displayName)
                .lineLimit(1)
            if shortcut.isStale {
                Text("App not found")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var recorder: some View {
        KeyboardShortcuts.Recorder(for: shortcut.keyboardShortcutName)
            .onChange(of: KeyboardShortcuts.getShortcut(for: shortcut.keyboardShortcutName)) {
                _, newValue in
                resolveConflicts(newShortcut: newValue)
                NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
            }
    }

    private var enableToggle: some View {
        Toggle("", isOn: Binding(
            get: { shortcut.isEnabled },
            set: { _ in
                store.toggleEnabled(id: shortcut.id)
            }
        ))
        .toggleStyle(.switch)
        .labelsHidden()
    }

    private func resolveConflicts(newShortcut: KeyboardShortcuts.Shortcut?) {
        guard let newShortcut else {
            conflictNotice = nil
            return
        }

        for other in store.shortcuts where other.id != shortcut.id {
            if let otherCombo = KeyboardShortcuts.getShortcut(for: other.keyboardShortcutName),
               otherCombo == newShortcut
            {
                KeyboardShortcuts.reset(other.keyboardShortcutName)
                conflictNotice = "Reassigned from \(other.displayName)"
                Task { @MainActor in
                    try? await Task.sleep(for: .seconds(3))
                    conflictNotice = nil
                }
                return
            }
        }
        conflictNotice = nil
    }
}

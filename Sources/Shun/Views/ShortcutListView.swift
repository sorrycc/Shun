import KeyboardShortcuts
import SwiftUI
import UniformTypeIdentifiers

struct ShortcutListView: View {
    @Environment(ShortcutStore.self) private var store
    @State private var selection: UUID?
    @State private var showingAppSearch = false
    @State private var pendingDeleteIds: [UUID] = []
    @State private var showDeleteConfirmation = false
    @State private var importAlertMessage: String?
    @State private var showImportAlert = false

    private let recipeService = RecipeService()

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
        .alert("Recipe Import", isPresented: $showImportAlert) {
            Button("OK") {}
        } message: {
            Text(importAlertMessage ?? "")
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
                    .frame(width: 24, height: 24)
            }

            Button(action: confirmRemoveSelected) {
                Image(systemName: "minus")
                    .frame(width: 24, height: 24)
            }
            .disabled(selection == nil)

            Spacer()

            Button(action: importRecipe) {
                Image(systemName: "square.and.arrow.down")
                    .frame(width: 24, height: 24)
            }
            .help("Import Recipe")

            Button(action: exportRecipe) {
                Image(systemName: "square.and.arrow.up")
                    .frame(width: 24, height: 24)
            }
            .disabled(store.shortcuts.isEmpty)
            .help("Export Recipe")
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
                    let url = URL(dataRepresentation: data, relativeTo: nil)
                else { return }

                Task { @MainActor in
                    if url.pathExtension == "hotrecipe" {
                        importRecipe(from: url)
                    } else if url.pathExtension == "app" {
                        addApp(from: url)
                    }
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

    // MARK: - Recipe Import/Export

    @MainActor
    private func importRecipe() {
        let panel = NSOpenPanel()
        panel.allowedContentTypes = [.init(filenameExtension: "hotrecipe")!]
        panel.allowsMultipleSelection = false
        guard panel.runModal() == .OK, let url = panel.url else { return }
        importRecipe(from: url)
    }

    @MainActor
    private func importRecipe(from url: URL) {
        do {
            let data = try Data(contentsOf: url)
            let recipe = try recipeService.importRecipe(from: data)
            let result = store.importRecipe(recipe)
            NotificationCenter.default.post(name: .shortcutsDidChange, object: nil)
            importAlertMessage = result.summary
            showImportAlert = true
        } catch {
            importAlertMessage = error.localizedDescription
            showImportAlert = true
        }
    }

    @MainActor
    private func exportRecipe() {
        let panel = NSSavePanel()
        panel.allowedContentTypes = [.init(filenameExtension: "hotrecipe")!]
        panel.nameFieldStringValue = "Shortcuts.hotrecipe"
        guard panel.runModal() == .OK, let url = panel.url else { return }
        do {
            let data = try recipeService.exportRecipe(from: store.shortcuts)
            try data.write(to: url, options: .atomic)
        } catch {
            importAlertMessage = "Export failed: \(error.localizedDescription)"
            showImportAlert = true
        }
    }
}

struct ShortcutRow: View {
    let shortcut: AppShortcut
    @Environment(ShortcutStore.self) private var store
    @Environment(UsageStore.self) private var usageStore
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
            } else {
                let count = usageStore.count(for: shortcut.bundleIdentifier)
                if count > 0 {
                    Text("\(count)× past 7 days")
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
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

import SwiftUI

struct InsightsView: View {
    @Environment(UsageStore.self) private var usageStore
    @Environment(ShortcutStore.self) private var store
    @State private var showClearConfirmation = false

    var body: some View {
        if usageStore.events.isEmpty {
            emptyState
        } else {
            statsContent
        }
    }

    private var emptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "chart.bar")
                .font(.system(size: 48))
                .foregroundStyle(.secondary)
            Text("No usage data yet")
                .font(.headline)
                .foregroundStyle(.secondary)
            Text("Start using your shortcuts — insights will appear here.")
                .font(.subheadline)
                .foregroundStyle(.tertiary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var statsContent: some View {
        Form {
            Section {
                HStack {
                    Image(systemName: "bolt.fill")
                        .foregroundStyle(.tint)
                    Text("You've used shortcuts ")
                        + Text("\(usageStore.totalCount) times").bold()
                        + Text(" in the past 7 days")
                }
                .padding(.vertical, 4)
            }

            Section("Most Used") {
                let topApps = usageStore.topApps(limit: 10)
                let maxCount = topApps.first?.count ?? 1
                ForEach(topApps, id: \.bundleId) { entry in
                    topAppRow(bundleId: entry.bundleId, count: entry.count, maxCount: maxCount)
                }
            }

            Section {
                Button("Clear Usage Data", role: .destructive) {
                    showClearConfirmation = true
                }
                .confirmationDialog(
                    "Clear all usage data?",
                    isPresented: $showClearConfirmation
                ) {
                    Button("Clear", role: .destructive) {
                        usageStore.clearAll()
                    }
                }
            }
        }
        .formStyle(.grouped)
        .padding()
    }

    private func topAppRow(bundleId: String, count: Int, maxCount: Int) -> some View {
        let shortcut = store.shortcuts.first { $0.bundleIdentifier == bundleId }
        let displayName = shortcut?.displayName ?? bundleId
        let icon: NSImage = if let url = shortcut?.bundleURL {
            AppScanner.shared.cachedIcon(for: url)
        } else {
            NSWorkspace.shared.icon(forFile: "/Applications")
        }

        return HStack(spacing: 10) {
            Image(nsImage: icon)
                .resizable()
                .frame(width: 24, height: 24)

            Text(displayName)
                .lineLimit(1)
                .frame(width: 120, alignment: .leading)

            GeometryReader { geo in
                RoundedRectangle(cornerRadius: 3)
                    .fill(Color.accentColor.opacity(0.6))
                    .frame(width: geo.size.width * CGFloat(count) / CGFloat(maxCount))
            }
            .frame(height: 14)

            Text("\(count)")
                .font(.caption)
                .foregroundStyle(.secondary)
                .monospacedDigit()
                .frame(width: 36, alignment: .trailing)
        }
        .padding(.vertical, 2)
    }
}

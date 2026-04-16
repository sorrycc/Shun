import Foundation
import Observation

@Observable @MainActor
final class UsageStore {
    private(set) var events: [UsageEvent] = []
    private let persistence: UsagePersistence
    private var saveTask: Task<Void, Never>?

    init(persistence: UsagePersistence = UsagePersistence()) {
        self.persistence = persistence
        load()
    }

    func log(bundleIdentifier: String) {
        events.append(UsageEvent(bundleIdentifier: bundleIdentifier, timestamp: Date()))
        save()
    }

    func load() {
        do {
            events = try persistence.load()
        } catch {
            events = []
        }
        pruneOldEvents()
    }

    func count(for bundleId: String) -> Int {
        events.count { $0.bundleIdentifier == bundleId }
    }

    var totalCount: Int {
        events.count
    }

    func topApps(limit: Int = 10) -> [(bundleId: String, count: Int)] {
        var counts: [String: Int] = [:]
        for event in events {
            counts[event.bundleIdentifier, default: 0] += 1
        }
        return counts
            .sorted { $0.value > $1.value }
            .prefix(limit)
            .map { (bundleId: $0.key, count: $0.value) }
    }

    func clearAll() {
        events = []
        saveTask?.cancel()
        try? persistence.delete()
    }

    func save() {
        saveTask?.cancel()
        saveTask = Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }
            self?.persistToDisk()
        }
    }

    func saveImmediately() {
        saveTask?.cancel()
        persistToDisk()
    }

    private func persistToDisk() {
        pruneOldEvents()
        do {
            try persistence.save(events)
        } catch {
            print("Failed to save usage data: \(error)")
        }
    }

    private func pruneOldEvents() {
        let cutoff = Date().addingTimeInterval(-7 * 24 * 60 * 60)
        events.removeAll { $0.timestamp < cutoff }
    }
}

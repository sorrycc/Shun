import Foundation
import Observation

@Observable @MainActor
final class UsageStore {
    private(set) var events: [UsageEvent] = []
    private let persistence: UsagePersistence
    private var saveTask: Task<Void, Never>?

    /// Collapse autorepeat bursts: keyboard repeats fire at ~10-15 Hz and the
    /// tap's autorepeat filter is unreliable for some hotkey combos, so the
    /// same bundle hit within this window is treated as one activation.
    private static let dedupWindow: TimeInterval = 0.5

    init(persistence: UsagePersistence = UsagePersistence()) {
        self.persistence = persistence
        load()
    }

    func log(bundleIdentifier: String) {
        let now = Date()
        if let last = events.last,
           last.bundleIdentifier == bundleIdentifier,
           now.timeIntervalSince(last.timestamp) < Self.dedupWindow {
            return
        }
        events.append(UsageEvent(bundleIdentifier: bundleIdentifier, timestamp: now))
        save()
    }

    func load() {
        do {
            events = try persistence.load()
        } catch {
            events = []
        }
        pruneOldEvents()
        dedupAutorepeatBursts()
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

    /// One-shot pass over the loaded array to remove same-bundle autorepeat
    /// bursts that pre-date the log()-time dedup. Events are chronologically
    /// ordered (only log() appends), so comparing against the last kept
    /// event is sufficient.
    private func dedupAutorepeatBursts() {
        var result: [UsageEvent] = []
        result.reserveCapacity(events.count)
        for event in events {
            if let last = result.last,
               last.bundleIdentifier == event.bundleIdentifier,
               event.timestamp.timeIntervalSince(last.timestamp) < Self.dedupWindow {
                continue
            }
            result.append(event)
        }
        events = result
    }
}

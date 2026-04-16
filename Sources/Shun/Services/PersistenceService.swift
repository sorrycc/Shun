import Foundation

/// Returns the app support directory for Shun, migrating the legacy "HotApp"
/// directory on first launch so existing users keep their shortcuts and usage history.
private func shunSupportDirectory() -> URL {
    let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
    let newDir = appSupport.appendingPathComponent("Shun", isDirectory: true)
    let legacyDir = appSupport.appendingPathComponent("HotApp", isDirectory: true)

    let fm = FileManager.default
    if !fm.fileExists(atPath: newDir.path),
       fm.fileExists(atPath: legacyDir.path) {
        try? fm.moveItem(at: legacyDir, to: newDir)
    }
    try? fm.createDirectory(at: newDir, withIntermediateDirectories: true)
    return newDir
}

protocol PersistenceService: Sendable {
    func load() throws -> [AppShortcut]
    func save(_ shortcuts: [AppShortcut]) throws
}

struct JSONFilePersistence: PersistenceService {
    static let defaultFileURL: URL = {
        shunSupportDirectory().appendingPathComponent("shortcuts.json")
    }()

    private let fileURL: URL

    init(fileURL: URL = Self.defaultFileURL) {
        self.fileURL = fileURL
    }

    func load() throws -> [AppShortcut] {
        guard FileManager.default.fileExists(atPath: fileURL.path) else {
            return []
        }
        let data = try Data(contentsOf: fileURL)
        return try JSONDecoder().decode([AppShortcut].self, from: data)
    }

    func save(_ shortcuts: [AppShortcut]) throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(shortcuts)
        try data.write(to: fileURL, options: .atomic)
    }
}

struct UsagePersistence: Sendable {
    static let defaultFileURL: URL = {
        shunSupportDirectory().appendingPathComponent("usage.json")
    }()

    private let fileURL: URL

    init(fileURL: URL = Self.defaultFileURL) {
        self.fileURL = fileURL
    }

    func load() throws -> [UsageEvent] {
        guard FileManager.default.fileExists(atPath: fileURL.path) else {
            return []
        }
        let data = try Data(contentsOf: fileURL)
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode([UsageEvent].self, from: data)
    }

    func save(_ events: [UsageEvent]) throws {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let data = try encoder.encode(events)
        try data.write(to: fileURL, options: .atomic)
    }

    func delete() throws {
        if FileManager.default.fileExists(atPath: fileURL.path) {
            try FileManager.default.removeItem(at: fileURL)
        }
    }
}

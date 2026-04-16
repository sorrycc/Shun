import Foundation
import KeyboardShortcuts

struct AppShortcut: Identifiable, Codable, Sendable {
    let id: UUID
    var bundleURL: URL
    var bundleIdentifier: String
    var displayName: String
    var shortcutName: String
    var isEnabled: Bool
    var isStale: Bool

    var keyboardShortcutName: KeyboardShortcuts.Name {
        KeyboardShortcuts.Name(shortcutName)
    }

    init(
        id: UUID = UUID(),
        bundleURL: URL,
        bundleIdentifier: String,
        displayName: String,
        isEnabled: Bool = true
    ) {
        self.id = id
        self.bundleURL = bundleURL
        self.bundleIdentifier = bundleIdentifier
        self.displayName = displayName
        self.shortcutName = "hotapp_\(id.uuidString)"
        self.isEnabled = isEnabled
        self.isStale = false
    }

    // MARK: - Codable

    enum CodingKeys: String, CodingKey {
        case id, bundleURL, bundleIdentifier, displayName, shortcutName, isEnabled
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(UUID.self, forKey: .id)
        bundleURL = try container.decode(URL.self, forKey: .bundleURL)
        bundleIdentifier = try container.decode(String.self, forKey: .bundleIdentifier)
        displayName = try container.decode(String.self, forKey: .displayName)
        shortcutName = try container.decode(String.self, forKey: .shortcutName)
        isEnabled = try container.decode(Bool.self, forKey: .isEnabled)
        isStale = false
    }

    // MARK: - Equatable / Hashable (exclude isStale)

    static func == (lhs: Self, rhs: Self) -> Bool {
        lhs.id == rhs.id
            && lhs.bundleURL == rhs.bundleURL
            && lhs.bundleIdentifier == rhs.bundleIdentifier
            && lhs.displayName == rhs.displayName
            && lhs.shortcutName == rhs.shortcutName
            && lhs.isEnabled == rhs.isEnabled
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(id)
        hasher.combine(bundleURL)
        hasher.combine(shortcutName)
        hasher.combine(isEnabled)
    }
}

extension AppShortcut: Equatable {}
extension AppShortcut: Hashable {}

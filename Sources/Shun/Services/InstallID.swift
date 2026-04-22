import Foundation

enum InstallID {
    private static let key = "telemetry.installId"

    static func current() -> String {
        if let existing = UserDefaults.standard.string(forKey: key), !existing.isEmpty {
            return existing
        }
        let new = UUID().uuidString
        UserDefaults.standard.set(new, forKey: key)
        return new
    }

    static func reset() {
        UserDefaults.standard.set(UUID().uuidString, forKey: key)
    }
}

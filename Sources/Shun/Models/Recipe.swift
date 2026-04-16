import Foundation

struct Recipe: Codable, Sendable {
    static let currentVersion = 1

    let version: Int
    let entries: [RecipeEntry]
}

struct RecipeEntry: Codable, Sendable {
    let bundleIdentifier: String
    let displayName: String
    let keyCode: Int
    let carbonModifiers: Int
}

enum RecipeError: LocalizedError {
    case unsupportedVersion(Int)

    var errorDescription: String? {
        switch self {
        case .unsupportedVersion(let version):
            return "This recipe requires a newer version of Shun (recipe version \(version))."
        }
    }
}

struct ImportResult: Sendable {
    var added: Int = 0
    var skippedDuplicate: Int = 0
    var skippedNotFound: Int = 0
    var skippedKeyConflict: Int = 0
    var total: Int { added + skippedDuplicate + skippedNotFound + skippedKeyConflict }

    var summary: String {
        if added == total {
            return "Imported all \(added) shortcuts."
        }
        var parts: [String] = []
        if skippedDuplicate > 0 { parts.append("\(skippedDuplicate) duplicate") }
        if skippedNotFound > 0 { parts.append("\(skippedNotFound) not found") }
        if skippedKeyConflict > 0 { parts.append("\(skippedKeyConflict) key conflict") }
        return "Imported \(added) of \(total) shortcuts (\(parts.joined(separator: ", ")))."
    }
}

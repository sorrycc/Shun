import Foundation

struct UsageEvent: Codable, Sendable {
    let bundleIdentifier: String
    let timestamp: Date
}

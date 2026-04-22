import Foundation

struct TelemetryConfig {
    let endpointURL: URL
    let ingestKey: String

    // Present only in packaged builds. Returns nil in `swift run` dev builds
    // (same gating pattern as Sparkle's SUFeedURL).
    static func fromBundle() -> TelemetryConfig? {
        guard
            let urlString = Bundle.main.object(forInfoDictionaryKey: "TelemetryEndpointURL") as? String,
            let url = URL(string: urlString),
            let ingestKey = Bundle.main.object(forInfoDictionaryKey: "TelemetryIngestKey") as? String,
            !ingestKey.isEmpty
        else { return nil }
        return TelemetryConfig(endpointURL: url, ingestKey: ingestKey)
    }

    var appID: String { "shun" }
    var appVersion: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown"
    }
    var osVersion: String {
        let v = ProcessInfo.processInfo.operatingSystemVersion
        return "\(v.majorVersion).\(v.minorVersion).\(v.patchVersion)"
    }
    var locale: String { Locale.current.identifier }
}

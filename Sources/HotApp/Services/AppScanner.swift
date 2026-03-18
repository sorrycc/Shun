import AppKit

struct ScannedApp: Identifiable, Sendable {
    let id: String  // bundleIdentifier
    let name: String
    let bundleURL: URL
    let bundleIdentifier: String
    let icon: NSImage

    @MainActor
    static func from(url: URL) -> ScannedApp? {
        guard let bundle = Bundle(url: url),
              let identifier = bundle.bundleIdentifier
        else { return nil }

        let name = bundle.object(forInfoDictionaryKey: "CFBundleName") as? String
            ?? bundle.object(forInfoDictionaryKey: "CFBundleDisplayName") as? String
            ?? url.deletingPathExtension().lastPathComponent

        let icon = NSWorkspace.shared.icon(forFile: url.path)
        icon.size = NSSize(width: 32, height: 32)

        return ScannedApp(
            id: identifier,
            name: name,
            bundleURL: url,
            bundleIdentifier: identifier,
            icon: icon
        )
    }
}

@MainActor
final class AppScanner {
    private var cache: [ScannedApp]?
    private var lastScanTime: Date?
    private let cacheTimeout: TimeInterval = 60

    static let searchDirectories: [URL] = {
        var dirs = [
            URL(fileURLWithPath: "/Applications"),
            URL(fileURLWithPath: "/System/Applications"),
        ]
        if let home = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Applications") as URL?
        {
            dirs.append(home)
        }
        return dirs
    }()

    var needsRefresh: Bool {
        guard let lastScanTime else { return true }
        return Date().timeIntervalSince(lastScanTime) > cacheTimeout
    }

    func scan() -> [ScannedApp] {
        if let cache, !needsRefresh {
            return cache
        }

        var apps: [ScannedApp] = []
        var seen = Set<String>()
        let fm = FileManager.default

        for directory in Self.searchDirectories {
            guard let contents = try? fm.contentsOfDirectory(
                at: directory,
                includingPropertiesForKeys: nil,
                options: [.skipsHiddenFiles]
            ) else { continue }

            for url in contents where url.pathExtension == "app" {
                if let app = ScannedApp.from(url: url), !seen.contains(app.bundleIdentifier) {
                    seen.insert(app.bundleIdentifier)
                    apps.append(app)
                }
            }
        }

        apps.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        cache = apps
        lastScanTime = Date()
        return apps
    }

    func invalidateCache() {
        cache = nil
        lastScanTime = nil
    }
}

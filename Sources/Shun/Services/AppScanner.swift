import AppKit

struct ScannedApp: Identifiable, Sendable {
    let id: String  // bundleIdentifier
    let name: String
    let bundleURL: URL
    let bundleIdentifier: String
    let icon: NSImage
}

@MainActor
final class AppScanner {
    static let shared = AppScanner()

    private var cache: [ScannedApp]?
    private var lastScanTime: Date?
    private let cacheTimeout: TimeInterval = 60
    private var iconCache: [String: NSImage] = [:]

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

    private var needsRefresh: Bool {
        guard let lastScanTime else { return true }
        return Date().timeIntervalSince(lastScanTime) > cacheTimeout
    }

    func scanAsync() async -> [ScannedApp] {
        if let cache, !needsRefresh {
            return cache
        }

        let directories = Self.searchDirectories

        // Directory enumeration + Bundle reads off main thread
        let entries: [(url: URL, name: String, bundleIdentifier: String)] = await Task.detached {
            var results: [(url: URL, name: String, bundleIdentifier: String)] = []
            var seen = Set<String>()
            let fm = FileManager.default

            for directory in directories {
                guard let contents = try? fm.contentsOfDirectory(
                    at: directory,
                    includingPropertiesForKeys: nil,
                    options: [.skipsHiddenFiles]
                ) else { continue }

                for url in contents where url.pathExtension == "app" {
                    guard let bundle = Bundle(url: url),
                          let identifier = bundle.bundleIdentifier,
                          !seen.contains(identifier)
                    else { continue }

                    seen.insert(identifier)

                    let name = bundle.object(forInfoDictionaryKey: "CFBundleName") as? String
                        ?? bundle.object(forInfoDictionaryKey: "CFBundleDisplayName") as? String
                        ?? url.deletingPathExtension().lastPathComponent

                    results.append((url: url, name: name, bundleIdentifier: identifier))
                }
            }

            return results
        }.value

        // Icon fetches on MainActor (AppKit requirement)
        var apps: [ScannedApp] = []
        for entry in entries {
            let icon = cachedIcon(for: entry.url)
            apps.append(ScannedApp(
                id: entry.bundleIdentifier,
                name: entry.name,
                bundleURL: entry.url,
                bundleIdentifier: entry.bundleIdentifier,
                icon: icon
            ))
        }

        apps.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        cache = apps
        lastScanTime = Date()
        return apps
    }

    func cachedIcon(for bundleURL: URL) -> NSImage {
        let path = bundleURL.path
        if let cached = iconCache[path] {
            return cached
        }
        let icon = NSWorkspace.shared.icon(forFile: path)
        icon.size = NSSize(width: 32, height: 32)
        iconCache[path] = icon
        return icon
    }

    func invalidateCache() {
        cache = nil
        lastScanTime = nil
        iconCache.removeAll()
    }
}

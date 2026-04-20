// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Shun",
    platforms: [
        .macOS(.v14),
    ],
    dependencies: [
        .package(url: "https://github.com/sindresorhus/KeyboardShortcuts", exact: "1.15.0"),
        .package(url: "https://github.com/sparkle-project/Sparkle", .upToNextMinor(from: "2.7.0")),
    ],
    targets: [
        .executableTarget(
            name: "Shun",
            dependencies: [
                "KeyboardShortcuts",
                "Sparkle",
            ],
            path: "Sources/Shun",
            swiftSettings: [
                .swiftLanguageMode(.v6),
            ]
        ),
    ]
)

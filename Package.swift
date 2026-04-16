// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Shun",
    platforms: [
        .macOS(.v14),
    ],
    dependencies: [
        .package(url: "https://github.com/sindresorhus/KeyboardShortcuts", exact: "1.15.0"),
    ],
    targets: [
        .executableTarget(
            name: "Shun",
            dependencies: [
                "KeyboardShortcuts",
            ],
            path: "Sources/Shun",
            swiftSettings: [
                .swiftLanguageMode(.v6),
            ]
        ),
    ]
)

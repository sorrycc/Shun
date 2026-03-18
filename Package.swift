// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "HotApp",
    platforms: [
        .macOS(.v14),
    ],
    dependencies: [
        .package(url: "https://github.com/sindresorhus/KeyboardShortcuts", exact: "1.15.0"),
    ],
    targets: [
        .executableTarget(
            name: "HotApp",
            dependencies: [
                "KeyboardShortcuts",
            ],
            path: "Sources/HotApp",
            swiftSettings: [
                .swiftLanguageMode(.v6),
            ]
        ),
    ]
)

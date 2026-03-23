// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "LeoRay",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "LeoRay",
            path: "Sources/LeoRay"
        )
    ]
)

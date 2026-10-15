// swift-tools-version:5.9
// The floating bar's native window for macOS (shipyard-bar), which
// Shipyard.app carries beside its own executable and starts when the bar is
// on (app/src-tauri/src/overlay.rs). See README.md.
import PackageDescription

let package = Package(
    name: "ShipyardBar",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "shipyard-bar",
            path: "Sources/ShipyardBar"
        ),
        .testTarget(
            name: "ShipyardBarTests",
            dependencies: ["shipyard-bar"],
            path: "Tests/ShipyardBarTests"
        ),
    ]
)

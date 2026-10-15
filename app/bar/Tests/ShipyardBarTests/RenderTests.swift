import SwiftUI
import XCTest
@testable import shipyard_bar

// Renders the bar's states to PNGs in $BAR_SNAPSHOTS, for a look by eye.
// Skipped unless that's set.
final class RenderTests: XCTestCase {
    @MainActor
    func testSnapshots() throws {
        guard let dir = ProcessInfo.processInfo.environment["BAR_SNAPSHOTS"] else { throw XCTSkip("BAR_SNAPSHOTS not set") }
        let cases: [(String, Place, Bool, Bool, CGSize)] = [
            ("edge-shut", .edge, false, false, CGSize(width: 380, height: 420)),
            ("edge-open", .edge, true, false, CGSize(width: 380, height: 420)),
            ("edge-card", .edge, true, true, CGSize(width: 380, height: 420)),
            ("notch-shut", .notch, false, false, CGSize(width: 640, height: 360)),
            ("notch-open", .notch, true, false, CGSize(width: 640, height: 360)),
            ("notch-card", .notch, true, true, CGSize(width: 640, height: 360)),
        ]
        for (name, place, open, card, size) in cases {
            let model = BarModel()
            model.apply(Demo.state)
            model.place = place
            model.notch = CGSize(width: 185, height: 32)
            model.open = open
            model.card = card
            let view = BarView(model: model)
                .frame(width: size.width, height: size.height)
                .background(LinearGradient(colors: [Color(red: 0.3, green: 0.45, blue: 0.6), Color(red: 0.75, green: 0.8, blue: 0.85)], startPoint: .top, endPoint: .bottom))
                .environment(\.colorScheme, .dark)
            let r = ImageRenderer(content: view)
            r.scale = 2
            guard let img = r.nsImage, let tiff = img.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff), let png = rep.representation(using: .png, properties: [:]) else { XCTFail("render \(name)"); continue }
            try png.write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(name).png"))
        }
    }
}

import SwiftUI
import XCTest
@testable import shipyard_bar

// Renders the bar's states to PNGs in $BAR_SNAPSHOTS, for a look by eye.
// Skipped unless that's set.
final class RenderTests: XCTestCase {
    @MainActor
    func testSnapshots() throws {
        guard let dir = ProcessInfo.processInfo.environment["BAR_SNAPSHOTS"] else { throw XCTSkip("BAR_SNAPSHOTS not set") }
        let cases: [(String, Place, Bool, Bool, CGSize, Message)] = [
            ("edge-shut", .edge, false, false, CGSize(width: 660, height: 420), Demo.state),
            ("edge-open", .edge, true, false, CGSize(width: 660, height: 420), Demo.state),
            ("edge-card", .edge, true, true, CGSize(width: 660, height: 420), Demo.state),
            ("notch-shut", .notch, false, false, CGSize(width: 700, height: 360), Demo.state),
            ("notch-open", .notch, true, false, CGSize(width: 700, height: 360), Demo.state),
            ("notch-card", .notch, true, true, CGSize(width: 700, height: 360), Demo.state),
            // More logins than fit: they scroll between the sign-in note and the agents.
            ("notch-card-many", .notch, true, true, CGSize(width: 700, height: 520), many),
            ("edge-card-many", .edge, true, true, CGSize(width: 660, height: 520), many),
        ]
        for (name, place, open, card, size, state) in cases {
            let model = BarModel()
            model.apply(state)
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

    // Six logins with three windows each, some unused: more than a card holds.
    private var many: Message {
        let now = Date().timeIntervalSince1970 * 1000
        let login = { (name: String, a: Double, b: Double, c: Double) -> String in
            #"{"agent":"claude","login":"\#(name)","name":"\#(name)","detail":"Acme · Claude Max · devl","windows":[{"name":"5h","minutes":300,"used":\#(a),"resets":\#(a > 0 ? now + 3_600_000 : 0)},{"name":"week","minutes":10080,"used":\#(b),"resets":\#(now + 86_400_000)},{"name":"week (Fable)","minutes":10080,"used":\#(c),"resets":\#(now + 86_400_000)}]}"#
        }
        let logins = [login("ana", 0, 0.29, 0.13), login("bo", 0, 0.36, 0), login("cy", 0, 0, 0), login("dee", 0.05, 0.30, 0), login("eli", 0, 0, 0), login("fay", 0.82, 0.91, 0.4)].joined(separator: ",")
        let json = #"{"type":"state","usage":true,"logins":[\#(logins)],"signedOut":["work (ana@acme.test)"],"waiting":[],"working":[]}"#
        return try! JSONDecoder().decode(Message.self, from: Data(json.utf8))
    }
}

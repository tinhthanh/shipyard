import Foundation
import SwiftUI

// What Shipyard sends the bar, one JSON object a line on stdin
// (app/src/overlay/control.ts builds it, overlay.rs passes it on):
//
//   {"type":"place","place":"edge"}       where the bar is: edge or notch
//   {"type":"state","usage":true,"logins":[…],"waiting":[…],"working":[…]}
//
// and what the bar sends back on stdout, when something on it is clicked:
//
//   {"type":"open","box":"devl","session":"shop-claude"}   a session
//   {"type":"open"}                                         Home

enum Place: String, Decodable {
    case edge, notch
}

struct Window: Decodable, Equatable, Hashable {
    var name: String
    var minutes: Double
    var used: Double
    // When it resets, in milliseconds since 1970.
    var resets: Double
}

struct Login: Decodable, Equatable, Identifiable {
    var agent: String
    var login: String
    var name: String
    var windows: [Window]
    var id: String { "\(agent):\(login)" }

    // The outer arc: the shortest window (Claude Code's session). The inner:
    // the most used of the others (a week).
    var outer: Window? { windows.first }
    var inner: Window? { windows.dropFirst().max(by: { $0.used < $1.used }) }
}

struct Agent: Decodable, Equatable, Identifiable {
    var key: String
    var box: String
    var session: String
    var agent: String
    var title: String
    var place: String
    var id: String { key }

    enum CodingKeys: String, CodingKey {
        case key, box, session, agent, title
        case place = "where"
    }
}

struct BarState: Decodable, Equatable {
    var usage: Bool = false
    var logins: [Login] = []
    var waiting: [Agent] = []
    var working: [Agent] = []
}

struct Message: Decodable {
    var type: String
    var place: Place?
    var usage: Bool?
    var logins: [Login]?
    var waiting: [Agent]?
    var working: [Agent]?
}

// The model the views draw and the controller moves.
final class BarModel: ObservableObject {
    @Published var place: Place = .edge
    @Published var state = BarState()
    // Open: the dock out of the edge, or the wings beside the notch.
    @Published var open = false
    // The card with the detail, beside the dock or under the notch.
    @Published var card = false
    // The display's notch, if it has one, and the menu bar's height.
    @Published var notch: CGSize?
    @Published var menuBar: CGFloat = 24
    // Where the shape and the card are drawn now, in the panel's top-left
    // coordinates: what takes the pointer, everything else lets it through.
    @Published var shapeFrame: CGRect = .zero
    @Published var cardFrame: CGRect = .zero

    func apply(_ m: Message) {
        switch m.type {
        case "place":
            if let p = m.place, p != place { place = p }
        case "state":
            let next = BarState(usage: m.usage ?? false, logins: m.logins ?? [], waiting: m.waiting ?? [], working: m.working ?? [])
            if next != state { state = next }
        default:
            break
        }
    }
}

// Clicks go back to Shipyard as a line on stdout.
enum Outbox {
    static func open(_ agent: Agent?) {
        var o: [String: String] = ["type": "open"]
        if let agent {
            o["box"] = agent.box
            o["session"] = agent.session
        }
        guard let data = try? JSONSerialization.data(withJSONObject: o, options: [.sortedKeys]) else { return }
        FileHandle.standardOutput.write(data + Data("\n".utf8))
    }
}

// "in 42m", "in 3h 5m", "in 2d 4h"
func resetsIn(_ ms: Double, now: Date = Date()) -> String {
    let m = max(0, Int(((ms / 1000) - now.timeIntervalSince1970) / 60 + 0.5))
    if m < 60 { return "in \(m)m" }
    let h = m / 60
    if h < 24 { return m % 60 == 0 ? "in \(h)h" : "in \(h)h \(m % 60)m" }
    return h % 24 == 0 ? "in \(h / 24)d" : "in \(h / 24)d \(h % 24)h"
}

// Amber at 70%, red at 90%: the plan's own warning colours.
func tone(_ used: Double) -> Color {
    used >= 0.9 ? Color(red: 0.97, green: 0.44, blue: 0.44) : used >= 0.7 ? Color(red: 0.98, green: 0.75, blue: 0.14) : Color(white: 0.92)
}

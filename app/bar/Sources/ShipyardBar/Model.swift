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

enum Place: String, Decodable, Sendable {
    case edge, notch
}

struct PlanWindow: Decodable, Equatable, Hashable, Sendable {
    var name: String
    var minutes: Double
    var used: Double
    // When it resets, in milliseconds since 1970; 0 for a window no request has started yet.
    var resets: Double
}

struct Login: Decodable, Equatable, Identifiable, Sendable {
    var agent: String
    var login: String
    var name: String
    // Whose organisation, which plan, on which boxes: "Acme · Claude Team · devl".
    var detail: String?
    var windows: [PlanWindow]
    var id: String { "\(agent):\(login)" }

    // The outer arc: the shortest window (Claude Code's session). The inner:
    // the most used of the others (a week).
    var outer: PlanWindow? { windows.first }
    var inner: PlanWindow? { windows.dropFirst().max(by: { $0.used < $1.used }) }

    // The windows the card lists: those with something used (at least 1%
    // shown), or the first alone when none has, so the login isn't empty.
    var shown: [PlanWindow] {
        let used = windows.filter { ($0.used * 100).rounded() >= 1 }
        return used.isEmpty ? Array(windows.prefix(1)) : used
    }
    // How close it is to a limit: its most used window.
    var most: Double { windows.map(\.used).max() ?? 0 }

    // Which to use first (Priority below).
    func priority(now: Date = Date()) -> Priority { Priority(self, now: now) }
}

// Priority says which login to use first, so that the least of a plan goes
// unused. What is lost is the week: what is left of it when it resets is
// gone, while a session (5h) comes back within hours. So:
//
//   need     what is left of the week over the hours until it resets: the
//            share an hour it must burn from now on to use it all. A week
//            ending soon with much left needs the most.
//   headroom what is left of the session now: a login whose 5h is nearly
//            spent can't take the work, however much its week needs it.
//   score    need × headroom; the highest goes first.
//
// A login with a window at 95% or more is full: it goes last, the one back
// soonest first. A model's own week ("week (Fable)") only limits that model,
// so it counts for neither. A window no request has started has its whole
// length ahead of it.
struct Priority: Equatable {
    static let full = 0.95

    var score: Double
    // % of the week to use an hour from now on, to use it all.
    var need: Double
    // What is left of the session (5h) now; 1 with none.
    var headroom: Double
    // When a full login is back, in ms since 1970; nil when it isn't full.
    var backAt: Double?

    init(_ l: Login, now: Date) {
        let at = now.timeIntervalSince1970 * 1000
        let plain = l.windows.filter { !$0.name.contains("(") }
        let pool = plain.isEmpty ? l.windows : plain
        let week = pool.max { $0.minutes != $1.minutes ? $0.minutes < $1.minutes : $0.used < $1.used }
        let session = pool.min { $0.minutes < $1.minutes }.flatMap { s in week.map { s.minutes < $0.minutes } == true ? s : nil }
        let hours = { (w: PlanWindow) in w.resets > 0 ? max(0.25, (w.resets - at) / 3_600_000) : w.minutes / 60 }
        need = week.map { max(0, 1 - $0.used) / hours($0) } ?? 0
        headroom = session.map { max(0, 1 - $0.used) } ?? 1
        score = need * headroom
        let blocking = pool.filter { $0.used >= Self.full }
        backAt = blocking.isEmpty ? nil : blocking.map { $0.resets > 0 ? $0.resets : at }.max()
    }

    // First to use first: the open by score, then the full by when they're back.
    static func before(_ a: Priority, _ b: Priority) -> Bool? {
        switch (a.backAt, b.backAt) {
        case (nil, .some): return true
        case (.some, nil): return false
        case let (.some(x), .some(y)): return x != y ? x < y : nil
        case (nil, nil): return a.score != b.score ? a.score > b.score : nil
        }
    }
}

struct Agent: Decodable, Equatable, Identifiable, Sendable {
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

struct BarState: Decodable, Equatable, Sendable {
    var usage: Bool = false
    var logins: [Login] = []
    // Logins Claude Code lost on a box, to sign in again: "ana-max (ana@acme.test)".
    var signedOut: [String] = []
    var waiting: [Agent] = []
    var working: [Agent] = []

    // The card's order: the login to use first first (Priority); ties keep
    // Shipyard's order (the rings keep it always).
    func cardLogins(now: Date = Date()) -> [(login: Login, priority: Priority)] {
        logins.enumerated().map { (i: $0.offset, l: $0.element, p: $0.element.priority(now: now)) }
            .sorted { a, b in Priority.before(a.p, b.p) ?? (a.i < b.i) }
            .map { ($0.l, $0.p) }
    }
}

struct Message: Decodable, Sendable {
    var type: String
    var place: Place?
    var usage: Bool?
    var logins: [Login]?
    var signedOut: [String]?
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
            let next = BarState(usage: m.usage ?? false, logins: m.logins ?? [], signedOut: m.signedOut ?? [], waiting: m.waiting ?? [], working: m.working ?? [])
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

import XCTest
@testable import shipyard_bar

final class ModelTests: XCTestCase {
    func testAStateLineDecodesWithWhereAsPlace() throws {
        let line = #"{"type":"state","usage":true,"logins":[{"agent":"claude","login":"a@x","name":"a","windows":[{"name":"5h","minutes":300,"used":0.3,"resets":1},{"name":"week","minutes":10080,"used":0.1,"resets":2},{"name":"week (Fable)","minutes":10080,"used":0.6,"resets":3}]}],"waiting":[{"key":"b/s","box":"b","session":"s","agent":"claude","title":"T","where":"shop · b"}],"working":[]}"#
        let m = try JSONDecoder().decode(Message.self, from: Data(line.utf8))
        let model = BarModel()
        model.apply(m)
        XCTAssertEqual(model.state.waiting.first?.place, "shop · b")
        XCTAssertEqual(model.state.logins.first?.outer?.name, "5h")
        // The inner arc is the most used of the others.
        XCTAssertEqual(model.state.logins.first?.inner?.name, "week (Fable)")
    }

    func testPlaceMessagesMoveTheBar() throws {
        let model = BarModel()
        model.apply(try JSONDecoder().decode(Message.self, from: Data(#"{"type":"place","place":"notch"}"#.utf8)))
        XCTAssertEqual(model.place, .notch)
    }

    func testResetsIn() {
        let now = Date(timeIntervalSince1970: 1_000_000)
        let at = { (minutes: Double) in (1_000_000 + minutes * 60) * 1000 }
        XCTAssertEqual(resetsIn(at(42), now: now), "in 42m")
        XCTAssertEqual(resetsIn(at(185), now: now), "in 3h 5m")
        XCTAssertEqual(resetsIn(at(120), now: now), "in 2h")
        XCTAssertEqual(resetsIn(at(52 * 60), now: now), "in 2d 4h")
    }

    func testTheCardListsUsedWindowsAndTheBusiestLoginFirst() {
        let w = { (n: String, used: Double, resets: Double) in PlanWindow(name: n, minutes: 300, used: used, resets: resets) }
        let idle = Login(agent: "claude", login: "idle", name: "idle", windows: [w("5h", 0, 0), w("week", 0.004, 1), w("week (Fable)", 0, 1)])
        let busy = Login(agent: "claude", login: "busy", name: "busy", windows: [w("5h", 0, 1), w("week", 0.36, 1), w("week (Fable)", 0, 1)])
        let some = Login(agent: "codex", login: "some", name: "some", windows: [w("5h", 0.12, 1), w("week", 0.29, 1)])
        // Nothing used: its first window alone, so it isn't empty.
        XCTAssertEqual(idle.shown.map(\.name), ["5h"])
        XCTAssertEqual(busy.shown.map(\.name), ["week"])
        XCTAssertEqual(some.shown.map(\.name), ["5h", "week"])
        var s = BarState()
        s.logins = [idle, some, busy]
        // The rings keep Shipyard's order.
        XCTAssertEqual(s.logins.map(\.login), ["idle", "some", "busy"])
    }

    // The card's logins from a real afternoon: the one whose week ends
    // soonest with most left goes first; a nearly spent 5h holds one back;
    // a full week goes last, whatever else it has.
    func testTheCardPutsTheLoginToUseFirstOnTop() {
        let now = Date(timeIntervalSince1970: 2_000_000)
        let at = { (h: Double) in (2_000_000 + h * 3600) * 1000 }
        let five = { (used: Double, h: Double) in PlanWindow(name: "5h", minutes: 300, used: used, resets: at(h)) }
        let week = { (used: Double, h: Double) in PlanWindow(name: "week", minutes: 10080, used: used, resets: at(h)) }
        let login = { (n: String, w: [PlanWindow]) in Login(agent: "claude", login: n, name: n, windows: w) }
        var s = BarState()
        s.logins = [
            login("full", [week(0.96, 25)]),
            login("half5h", [five(0.59, 2), week(0.45, 102)]),
            login("slow", [week(0.54, 116)]),
            login("spent5h", [five(0.97, 1), week(0.06, 72)]),
            login("soon", [week(0.10, 33), PlanWindow(name: "week (Fable)", minutes: 10080, used: 0.14, resets: at(33))]),
            login("fresh", [five(0.02, 4.5), week(0.13, 138)]),
            login("nhat", [five(0.04, 3.3), week(0.01, 41)]),
        ]
        let order = s.cardLogins(now: now)
        XCTAssertEqual(order.map(\.login.login), ["soon", "nhat", "fresh", "slow", "half5h", "spent5h", "full"])
        // 90% left over 33h.
        XCTAssertEqual(order[0].priority.need, 0.9 / 33, accuracy: 1e-9)
        // Full: back when the 5h resets, before the week that is.
        XCTAssertEqual(order[5].priority.backAt, at(1))
        XCTAssertEqual(order[6].priority.backAt, at(25))
        // A window no request has started has its whole length ahead.
        let unstarted = login("u", [PlanWindow(name: "week", minutes: 10080, used: 0, resets: 0)]).priority(now: now)
        XCTAssertEqual(unstarted.need, 1 / 168, accuracy: 1e-9)
    }

    func testTheDockGrowsWithWhatItShows() {
        var s = BarState()
        let empty = Layout.dockHeight(s)
        s.logins = [Login(agent: "claude", login: "a", name: "a", windows: []), Login(agent: "codex", login: "b", name: "b", windows: [])]
        s.waiting = [Agent(key: "k", box: "b", session: "s", agent: "claude", title: "t", place: "p")]
        XCTAssertGreaterThan(Layout.dockHeight(s), empty)
    }
}

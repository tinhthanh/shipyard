import AppKit

// shipyard-bar [--place edge|notch]
//
// Shipyard starts it when the floating bar is on and talks to it over stdin
// and stdout (Model.swift). It has no Dock icon and no menu, and it quits
// when Shipyard closes its stdin (or Shipyard itself goes away).
@main
enum ShipyardBar {
    @MainActor static var controller: BarController?

    static func main() {
        let app = NSApplication.shared
        app.setActivationPolicy(.accessory)
        let args = CommandLine.arguments
        let place = args.firstIndex(of: "--place").flatMap { i in i + 1 < args.count ? Place(rawValue: args[i + 1]) : nil } ?? .edge
        MainActor.assumeIsolated {
            let c = BarController()
            controller = c
            c.start(place: place)
            if args.contains("--demo") { c.model.apply(Demo.state) }
        }
        readCommands()
        app.run()
    }

    // One JSON object a line; the end of stdin is the end of the bar.
    static func readCommands() {
        let thread = Thread {
            let decoder = JSONDecoder()
            while let line = readLine(strippingNewline: true) {
                guard let data = line.data(using: .utf8), let message = try? decoder.decode(Message.self, from: data) else { continue }
                DispatchQueue.main.async {
                    MainActor.assumeIsolated { controller?.model.apply(message) }
                }
            }
            DispatchQueue.main.async { NSApp.terminate(nil) }
        }
        thread.start()
    }
}

// --demo: made-up readings, for trying the bar on its own.
enum Demo {
    static var state: Message {
        let now = Date().timeIntervalSince1970 * 1000
        let json = """
        {"type":"state","usage":true,
         "logins":[{"agent":"claude","login":"ana@acme.test","name":"ana","windows":[{"name":"5h","minutes":300,"used":0.34,"resets":\(now + 2.2 * 3_600_000)},{"name":"week (Fable)","minutes":10080,"used":0.72,"resets":\(now + 3 * 86_400_000)}]},
                   {"agent":"codex","login":"default","name":"default","windows":[{"name":"5h","minutes":300,"used":0.12,"resets":\(now + 4 * 3_600_000)},{"name":"week","minutes":10080,"used":0.41,"resets":\(now + 5 * 86_400_000)}]}],
         "waiting":[{"key":"devl/shop-claude","box":"devl","session":"shop-claude","agent":"claude","title":"Fix checkout webhook retries","where":"shop · devl"}],
         "working":[{"key":"gpu/judge","box":"gpu","session":"judge","agent":"codex","title":"Tune the judge prompt","where":"evals · gpu"}]}
        """
        return try! JSONDecoder().decode(Message.self, from: Data(json.utf8))
    }
}

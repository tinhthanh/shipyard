import SwiftUI

// What the bar draws. The panel is larger than anything on it (room for the
// card), fixed in place; only the shapes and their contents move, and only
// they take the pointer (BarHostingView).

enum Layout {
    // The edge dock, open: as wide as a ring and its margins.
    static let dockWidth: CGFloat = 54
    // The sliver it folds into.
    static let sliver = CGSize(width: 6, height: 64)
    static let ring: CGFloat = 32
    static let ringGap: CGFloat = 10
    static let count: CGFloat = 24
    // The card: the logins on the left, the agents on the right.
    static let loginsColumn: CGFloat = 288
    static let agentsColumn: CGFloat = 236
    static let cardPad: CGFloat = 12
    static let columnGap: CGFloat = 12
    static let card: CGFloat = loginsColumn + agentsColumn + columnGap * 2 + 0.5 + cardPad * 2
    static let gap: CGFloat = 10
    // The notch bar's wings, either side of the notch.
    static let wingPad: CGFloat = 12
    static let notchRing: CGFloat = 22
    // A screen without a notch: the bar folds into a lip under the top edge.
    static let lip = CGSize(width: 120, height: 5)
    // Room around the card for its shadow, which takes the pointer too, so
    // a click there isn't lost in a pixel that is neither card nor screen.
    static let cardShadow: CGFloat = 22

    static func dockHeight(_ s: BarState) -> CGFloat {
        let rings = max(1, s.logins.count)
        let counts = (s.waiting.isEmpty ? 0 : 1) + (s.working.isEmpty ? 0 : 1)
        var h: CGFloat = 16 * 2 + CGFloat(rings) * ring + CGFloat(rings - 1) * ringGap
        if counts > 0 { h += 12 + CGFloat(counts) * count + CGFloat(counts - 1) * 6 }
        return max(h, 72)
    }

    static func wing(_ s: BarState) -> CGFloat {
        let rings = max(1, s.logins.count)
        let left = CGFloat(rings) * notchRing + CGFloat(rings - 1) * 6
        let counts = (s.waiting.isEmpty ? 0 : 1) + (s.working.isEmpty ? 0 : 1)
        let right = CGFloat(counts) * 36
        return max(left, right, 28) + wingPad * 2
    }
}

let dockFill = Color(white: 0.085).opacity(0.97)

struct CardFrameKey: PreferenceKey {
    static var defaultValue: CGRect = .zero
    static func reduce(value: inout CGRect, nextValue: () -> CGRect) {
        let n = nextValue()
        if n != .zero { value = n }
    }
}

struct BarView: View {
    @ObservedObject var model: BarModel
    @Environment(\.accessibilityReduceMotion) private var reduce

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .topLeading) {
                if model.place == .edge { edge(geo.size) } else { notch(geo.size) }
            }
            .frame(width: geo.size.width, height: geo.size.height, alignment: .topLeading)
            .coordinateSpace(name: "panel")
            .onPreferenceChange(CardFrameKey.self) { model.cardFrame = model.card ? $0 : .zero }
        }
        .preferredColorScheme(.dark)
    }

    private func motion(_ a: Animation) -> Animation? { reduce ? nil : a }

    // MARK: the right edge

    @ViewBuilder private func edge(_ size: CGSize) -> some View {
        let h = Layout.dockHeight(model.state)
        let w = model.open ? Layout.dockWidth : Layout.sliver.width
        let shapeH = model.open ? h : Layout.sliver.height
        let dock = CGRect(x: size.width - w, y: (size.height - shapeH) / 2, width: w, height: shapeH)
        DockShape(width: w, height: shapeH)
            .fill(dockFill)
            .overlay(DockShape(width: w, height: shapeH).stroke(Color.white.opacity(model.open ? 0.08 : 0.16), lineWidth: 0.5))
            .overlay(alignment: .trailing) { sliverTone.opacity(model.open ? 0 : 1).frame(width: 2.5, height: 30).clipShape(Capsule()).padding(.trailing, 1.5) }
            .frame(width: size.width, height: size.height)
            .animation(motion(Motion.unfold), value: model.open)
            .animation(motion(Motion.unfold), value: h)
            .onAppear { model.shapeFrame = dock }
            .onChange(of: dock) { model.shapeFrame = $0 }

        VStack(spacing: 0) {
            Rings(logins: model.state.logins, size: Layout.ring, spacing: Layout.ringGap, open: model.open, vertical: true)
            if !model.state.waiting.isEmpty || !model.state.working.isEmpty {
                Counts(state: model.state, vertical: true, open: model.open).padding(.top, 12)
            }
        }
        .frame(width: Layout.dockWidth, height: h)
        .opacity(model.open ? 1 : 0)
        .offset(x: model.open ? 0 : 18)
        .animation(motion(model.open ? Motion.contents.delay(0.06) : Motion.contents), value: model.open)
        .position(x: size.width - Layout.dockWidth / 2 - 2, y: size.height / 2)

        if model.card {
            // Offered the panel's height less the shadow's room: the logins
            // scroll when the card would be taller.
            Card(state: model.state)
                .background(GeometryReader { g in Color.clear.preference(key: CardFrameKey.self, value: g.frame(in: .named("panel"))) })
                .frame(height: size.height - Layout.cardShadow * 2)
                .position(x: size.width - Layout.dockWidth - Layout.gap - Layout.card / 2, y: size.height / 2)
                .transition(.asymmetric(insertion: .opacity.combined(with: .offset(x: 14)).combined(with: .scale(scale: 0.97, anchor: .trailing)), removal: .opacity))
        }
    }

    private var sliverTone: Color {
        !model.state.waiting.isEmpty ? Color(red: 0.98, green: 0.75, blue: 0.14) : !model.state.working.isEmpty ? Color(red: 0.22, green: 0.74, blue: 0.97) : Color.white.opacity(0.35)
    }

    // MARK: the notch

    @ViewBuilder private func notch(_ size: CGSize) -> some View {
        let hole = model.notch
        let depth = hole?.height ?? model.menuBar
        let wing = Layout.wing(model.state)
        let shut = hole.map { CGSize(width: $0.width, height: $0.height) } ?? Layout.lip
        let open = CGSize(width: (hole?.width ?? 12) + wing * 2, height: max(depth, 30))
        let s = model.open ? open : shut
        let frame = CGRect(x: (size.width - s.width) / 2, y: 0, width: s.width, height: max(s.height, 8))
        NotchShape(width: s.width, height: s.height)
            .fill(Color.black)
            .frame(width: size.width, height: size.height, alignment: .top)
            .animation(motion(Motion.unfold), value: model.open)
            .animation(motion(Motion.unfold), value: wing)
            .onAppear { model.shapeFrame = frame }
            .onChange(of: frame) { model.shapeFrame = $0 }

        HStack(spacing: 0) {
            Rings(logins: model.state.logins, size: Layout.notchRing, spacing: 6, open: model.open, vertical: false)
                .frame(width: wing, alignment: .trailing)
                .padding(.trailing, 0)
            Color.clear.frame(width: hole?.width ?? 12)
            Counts(state: model.state, vertical: false, open: model.open)
                .frame(width: wing, alignment: .leading)
        }
        .frame(height: open.height)
        .opacity(model.open ? 1 : 0)
        .offset(y: model.open ? 0 : -8)
        .animation(motion(model.open ? Motion.contents.delay(0.06) : Motion.contents), value: model.open)
        .position(x: size.width / 2, y: open.height / 2)

        if model.card {
            VStack(spacing: 0) {
                Color.clear.frame(height: open.height + Layout.gap)
                Card(state: model.state)
                    .background(GeometryReader { g in Color.clear.preference(key: CardFrameKey.self, value: g.frame(in: .named("panel"))) })
                    .transition(.asymmetric(insertion: .opacity.combined(with: .offset(y: -10)).combined(with: .scale(scale: 0.97, anchor: .top)), removal: .opacity))
            }
            // The shadow's room below; the card is offered the rest, and its
            // logins scroll when it would be taller.
            .frame(width: size.width, height: size.height - Layout.cardShadow, alignment: .top)
        }
    }
}

// A ring for each login: the outer arc its shortest window, the inner the
// most used other one. Each comes in a beat after the one before.
struct Rings: View {
    var logins: [Login]
    var size: CGFloat
    var spacing: CGFloat
    var open: Bool
    var vertical: Bool

    var body: some View {
        let stack = Group {
            if logins.isEmpty {
                Image(systemName: "sailboat").font(.system(size: size * 0.45)).foregroundStyle(.white.opacity(0.55)).frame(width: size, height: size)
            }
            ForEach(Array(logins.enumerated()), id: \.element.id) { i, l in
                Button { Outbox.open(nil) } label: { Ring(login: l, size: size) }
                    .buttonStyle(.plain)
                    .help("\(l.name)" + (l.detail.map { " (\($0))" } ?? "") + ": " + l.windows.map { "\($0.name) \(Int(($0.used * 100).rounded()))%" }.joined(separator: ", "))
                    .scaleEffect(open ? 1 : 0.6)
                    .opacity(open ? 1 : 0)
                    .animation(Motion.stagger(i), value: open)
            }
        }
        if vertical { VStack(spacing: spacing) { stack } } else { HStack(spacing: spacing) { stack } }
    }
}

struct Ring: View {
    var login: Login
    var size: CGFloat

    var body: some View {
        let line = max(2, size / 11)
        ZStack {
            arc(login.outer, inset: 0, line: line)
            arc(login.inner, inset: line + 2.5, line: line * 0.85)
            AgentGlyph(agent: login.agent).font(.system(size: size * 0.32, weight: .bold))
        }
        .frame(width: size, height: size)
        .accessibilityLabel("\(login.name): " + login.windows.map { "\($0.name) \(Int(($0.used * 100).rounded())) percent" }.joined(separator: ", "))
    }

    @ViewBuilder private func arc(_ w: PlanWindow?, inset: CGFloat, line: CGFloat) -> some View {
        if let w {
            ZStack {
                Circle().stroke(Color.white.opacity(0.14), lineWidth: line)
                Circle()
                    .trim(from: 0, to: min(1, max(0.015, w.used)))
                    .stroke(tone(w.used), style: StrokeStyle(lineWidth: line, lineCap: .round))
                    .rotationEffect(.degrees(-90))
                    .animation(Motion.reading, value: w.used)
            }
            .padding(inset + line / 2)
        }
    }
}

struct AgentGlyph: View {
    var agent: String
    var body: some View {
        switch agent {
        case "claude": Image(systemName: "asterisk").foregroundStyle(Color(red: 0.85, green: 0.47, blue: 0.34))
        case "codex": Image(systemName: "hexagon").foregroundStyle(.white.opacity(0.85))
        case "gemini": Image(systemName: "sparkle").foregroundStyle(Color(red: 0.44, green: 0.61, blue: 1))
        default: Image(systemName: "terminal").foregroundStyle(.white.opacity(0.7))
        }
    }
}

// Agents that need you (an amber dot) and working (a turning arc).
struct Counts: View {
    var state: BarState
    var vertical: Bool
    var open: Bool

    var body: some View {
        let items = Group {
            if !state.waiting.isEmpty {
                Button { Outbox.open(state.waiting.first) } label: {
                    HStack(spacing: 4) {
                        Circle().fill(Color(red: 0.98, green: 0.75, blue: 0.14)).frame(width: 7, height: 7)
                        Text("\(state.waiting.count)").font(.system(size: 11, weight: .semibold).monospacedDigit()).foregroundStyle(Color(red: 0.99, green: 0.83, blue: 0.4))
                    }
                    .frame(height: Layout.count)
                }
                .buttonStyle(.plain)
                .help("\(state.waiting.count) need you")
            }
            if !state.working.isEmpty {
                Button { Outbox.open(state.working.first) } label: {
                    HStack(spacing: 4) {
                        Spinner(active: open).frame(width: 9, height: 9)
                        Text("\(state.working.count)").font(.system(size: 11, weight: .semibold).monospacedDigit()).foregroundStyle(Color(red: 0.49, green: 0.83, blue: 0.99))
                    }
                    .frame(height: Layout.count)
                }
                .buttonStyle(.plain)
                .help("\(state.working.count) working")
            }
        }
        if vertical { VStack(spacing: 6) { items } } else { HStack(spacing: 8) { items } }
    }
}

// It turns only while the bar is open: shut, nothing on the bar redraws.
struct Spinner: View {
    var active: Bool
    @Environment(\.accessibilityReduceMotion) private var reduce
    var body: some View {
        TimelineView(.animation(paused: !active || reduce)) { t in
            Circle()
                .trim(from: 0, to: 0.7)
                .stroke(Color(red: 0.22, green: 0.74, blue: 0.97), style: StrokeStyle(lineWidth: 1.6, lineCap: .round))
                .rotationEffect(.degrees(t.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 0.9) / 0.9 * 360))
        }
    }
}

// The card: each login's windows with when they reset, the one to use first
// on top, beside the agents that need you and those working, a click from
// opening.
struct Card: View {
    // Takes the height it needs, or what it is offered when that is less:
    // the logins scroll in what the sign-in note leaves.
    var state: BarState

    var body: some View {
        HStack(alignment: .top, spacing: Layout.columnGap) {
            logins.frame(width: Layout.loginsColumn, alignment: .topLeading)
            Color.clear.frame(width: 0.5, height: 0)
            agents.frame(width: Layout.agentsColumn, alignment: .topLeading)
        }
        // The rule between the columns, as tall as the taller of them.
        .overlay(alignment: .leading) {
            Rectangle().fill(Color.white.opacity(0.08)).frame(width: 0.5).padding(.leading, Layout.loginsColumn + Layout.columnGap)
        }
        .padding(Layout.cardPad)
        .frame(width: Layout.card, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 14, style: .continuous).fill(dockFill))
        .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(Color.white.opacity(0.09), lineWidth: 0.5))
        .shadow(color: .black.opacity(0.35), radius: 16, y: 6)
    }

    private var logins: some View {
        VStack(alignment: .leading, spacing: 10) {
            if !state.signedOut.isEmpty {
                Button { Outbox.open(nil) } label: {
                    HStack(alignment: .top, spacing: 6) {
                        Image(systemName: "person.crop.circle.badge.exclamationmark").font(.system(size: 11))
                        Text("Signed out: \(state.signedOut.joined(separator: ", ")). Sign in again in Shipyard's Usage → Accounts.")
                            .font(.system(size: 11)).fixedSize(horizontal: false, vertical: true)
                    }
                    .foregroundStyle(Color(red: 0.98, green: 0.75, blue: 0.14))
                }
                .buttonStyle(.plain)
            }
            if state.logins.isEmpty && state.signedOut.isEmpty {
                Text(state.usage ? "No plan limits read yet." : "Turn on the Usage & accounts plugin in Shipyard to see plan limits here.")
                    .font(.system(size: 11)).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            }
            if !state.logins.isEmpty {
                // Ordered as the card opens, and as new readings come.
                Logins(logins: state.cardLogins())
            }
        }
    }

    private var agents: some View {
        VStack(alignment: .leading, spacing: 10) {
            section("Need you", state.waiting, Color(red: 0.98, green: 0.75, blue: 0.14))
            if !state.waiting.isEmpty && !state.working.isEmpty { Divider().overlay(Color.white.opacity(0.08)) }
            section("Working", state.working, Color(red: 0.22, green: 0.74, blue: 0.97))
            if state.waiting.isEmpty && state.working.isEmpty {
                Text("No agent is working.").font(.system(size: 11)).foregroundStyle(.secondary)
            }
        }
    }

    // As many as the column has room for beside the logins, near enough.
    private static let rows = 10

    @ViewBuilder private func section(_ title: String, _ rows: [Agent], _ dot: Color) -> some View {
        if !rows.isEmpty {
            VStack(alignment: .leading, spacing: 2) {
                Text("\(title) · \(rows.count)").font(.system(size: 11, weight: .medium)).foregroundStyle(.secondary)
                ForEach(rows.prefix(Self.rows)) { a in
                    Button { Outbox.open(a) } label: {
                        HStack(alignment: .firstTextBaseline, spacing: 6) {
                            Circle().fill(dot).frame(width: 5, height: 5).alignmentGuide(.firstTextBaseline) { $0[.bottom] }
                            AgentGlyph(agent: a.agent).font(.system(size: 9, weight: .bold))
                            VStack(alignment: .leading, spacing: 1) {
                                Text(a.title).font(.system(size: 11)).lineLimit(1)
                                Text(a.place).font(.system(size: 9)).foregroundStyle(.tertiary).lineLimit(1).truncationMode(.middle)
                            }
                            Spacer(minLength: 0)
                        }
                        .padding(.vertical, 3)
                        .padding(.horizontal, 4)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(RowStyle())
                }
                if rows.count > Self.rows {
                    Text("+\(rows.count - Self.rows) more").font(.system(size: 9)).foregroundStyle(.tertiary).padding(.leading, 4)
                }
            }
        }
    }
}

struct RowStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(RoundedRectangle(cornerRadius: 6).fill(Color.white.opacity(configuration.isPressed ? 0.14 : 0)))
    }
}

// The card's logins, in a scroll view as tall as they are, or as tall as the
// card has room for when they don't all fit.
struct Logins: View {
    var logins: [(login: Login, priority: Priority)]
    @State private var height: CGFloat = 0

    var body: some View {
        ScrollView(.vertical, showsIndicators: false) {
            VStack(alignment: .leading, spacing: 10) {
                ForEach(Array(logins.enumerated()), id: \.element.login.id) { i, l in
                    LoginRows(login: l.login, priority: l.priority, rank: i + 1)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(GeometryReader { g in Color.clear.preference(key: ContentHeightKey.self, value: g.size.height) })
        }
        .frame(maxHeight: height)
        .onPreferenceChange(ContentHeightKey.self) { height = $0 }
    }
}

struct ContentHeightKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) { value = max(value, nextValue()) }
}

struct LoginRows: View {
    var login: Login
    var priority: Priority
    // Its place in the order to use them, from 1.
    var rank: Int

    var body: some View {
        let l = login
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 6) {
                AgentGlyph(agent: l.agent).font(.system(size: 10, weight: .bold))
                Text(l.name).font(.system(size: 12, weight: .medium)).lineLimit(1)
                Spacer(minLength: 4)
                tag
            }
            if let d = l.detail, !d.isEmpty {
                Text(d).font(.system(size: 10)).foregroundStyle(.secondary).lineLimit(1).truncationMode(.tail)
            }
            ForEach(l.shown, id: \.self) { w in
                VStack(spacing: 3) {
                    HStack(spacing: 6) {
                        Text(w.name).font(.system(size: 11)).lineLimit(1)
                        Spacer(minLength: 4)
                        TimelineView(.periodic(from: .now, by: 30)) { t in
                            // 0: a window no request has started yet.
                            Text(w.resets > 0 ? "resets \(resetsIn(w.resets, now: t.date))" : "not started").font(.system(size: 10)).foregroundStyle(.secondary)
                        }
                        Text("\(Int((w.used * 100).rounded()))%").font(.system(size: 11).monospacedDigit()).frame(width: 34, alignment: .trailing)
                    }
                    GeometryReader { g in
                        ZStack(alignment: .leading) {
                            Capsule().fill(Color.white.opacity(0.1))
                            Capsule().fill(tone(w.used)).frame(width: g.size.width * min(1, w.used))
                        }
                    }
                    .frame(height: 4)
                }
            }
        }
        // A full login can't take work now: it steps back.
        .opacity(priority.backAt == nil ? 1 : 0.55)
    }

    // "use next" on the first, its rank on the others, or when a full one
    // is back; its score, the share of its week an hour it needs held back
    // by what is left of its 5h, to say why.
    @ViewBuilder private var tag: some View {
        if let back = priority.backAt {
            TimelineView(.periodic(from: .now, by: 30)) { t in
                Text("full · back \(resetsIn(back, now: t.date))").font(.system(size: 9, weight: .medium)).foregroundStyle(Color(red: 0.97, green: 0.44, blue: 0.44))
            }
        } else {
            HStack(spacing: 5) {
                Text(String(format: "%.1f%%/h", priority.score * 100)).font(.system(size: 9).monospacedDigit()).foregroundStyle(.tertiary)
                if rank == 1 {
                    Text("use next").font(.system(size: 9, weight: .semibold))
                        .padding(.horizontal, 5).padding(.vertical, 1)
                        .background(Capsule().fill(Color(red: 0.2, green: 0.78, blue: 0.45).opacity(0.22)))
                        .foregroundStyle(Color(red: 0.45, green: 0.9, blue: 0.6))
                } else {
                    Text("#\(rank)").font(.system(size: 9, weight: .medium).monospacedDigit()).foregroundStyle(.secondary)
                }
            }
            .help(priority.headroom < 1
                ? String(format: "Its week needs %.1f%% an hour from now on to be used up before it resets; %.0f%% of its 5h is left.", priority.need * 100, priority.headroom * 100)
                : String(format: "Its week needs %.1f%% an hour from now on to be used up before it resets.", priority.need * 100))
        }
    }
}

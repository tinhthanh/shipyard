import AppKit
import Combine
import SwiftUI

// BarController keeps the panel where the bar belongs and opens and folds it
// under the pointer. After Codenotch's NotchWindowController (MIT): the panel
// never moves or resizes while it is in use, only what is drawn in it does,
// and the pointer is followed with a global mouse monitor (and a slow poll),
// so the bar opens under a pointer whatever app is in front, and with no
// window of its own having to be key.
@MainActor
final class BarController {
    let model = BarModel()
    private var panel: BarPanel?
    private var hosting: BarHostingView<BarView>?
    private var monitors: [Any] = []
    private var poll: Timer?
    private var bag = Set<AnyCancellable>()
    private var closing: DispatchWorkItem?
    private var dwelling: DispatchWorkItem?

    // How long the pointer may be away before the bar folds, and how long it
    // rests on the open bar before the card comes.
    static let grace = 0.22
    static let dwell = 0.28

    func start(place: Place) {
        model.place = place
        let panel = BarPanel(frame: .zero)
        let container = BarContainerView()
        let hosting = BarHostingView(rootView: BarView(model: model))
        hosting.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(hosting)
        NSLayoutConstraint.activate([
            hosting.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            hosting.trailingAnchor.constraint(equalTo: container.trailingAnchor),
            hosting.topAnchor.constraint(equalTo: container.topAnchor),
            hosting.bottomAnchor.constraint(equalTo: container.bottomAnchor),
        ])
        panel.contentView = container
        self.panel = panel
        self.hosting = hosting
        position()
        panel.orderFrontRegardless()

        model.$place.dropFirst().sink { [weak self] _ in
            DispatchQueue.main.async { self?.fold(); self?.position() }
        }.store(in: &bag)
        // More logins make the notch bar wider than its panel: widen it.
        model.$state.sink { [weak self] _ in
            DispatchQueue.main.async { self?.positionIfNarrow() }
        }.store(in: &bag)
        Publishers.CombineLatest3(model.$shapeFrame, model.$cardFrame, model.$card)
            .sink { [weak self] _, _, _ in DispatchQueue.main.async { self?.updateHits() } }
            .store(in: &bag)
        NotificationCenter.default.addObserver(forName: NSApplication.didChangeScreenParametersNotification, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.position() }
        }
        watchPointer()
    }

    // The screen with the menu bar: where the bar lives.
    private var screen: NSScreen? { NSScreen.screens.first }

    // The panel's fixed frame: on the edge, against its right side and
    // centred, with room on the left for the card; at the notch, from the top
    // and centred, with room below.
    func position() {
        guard let panel, let screen else { return }
        panel.setFrame(frame(on: screen), display: true)
        updateHits()
    }

    private func positionIfNarrow() {
        guard model.place == .notch, let panel, let screen else { return }
        if frame(on: screen).width > panel.frame.width { position() }
    }

    private func frame(on screen: NSScreen) -> CGRect {
        let f = screen.frame
        let visible = screen.visibleFrame
        let notch = Self.notch(of: screen)
        if model.notch != notch { model.notch = notch }
        let menuBar = max(0, f.maxY - visible.maxY)
        if model.menuBar != menuBar { model.menuBar = menuBar }
        switch model.place {
        case .edge:
            // Within the screen below the menu bar, so the card never runs
            // under it.
            let width = Layout.card + Layout.gap + Layout.dockWidth + Layout.cardShadow * 2
            let height = min(visible.height, 760)
            return CGRect(x: f.maxX - width, y: visible.midY - height / 2, width: width, height: height)
        case .notch:
            let open = (model.notch?.width ?? 12) + Layout.wing(model.state) * 2 + 40
            let width = max(640, open)
            let height = min(f.height * 0.8, 640)
            return CGRect(x: f.midX - width / 2, y: f.maxY - height, width: width, height: height)
        }
    }

    // The display's own notch, measured from the menu bar strips either side
    // of it; the deepest of those and the safe area is its depth (the safe
    // area collapses when the menu bar hides, the hole does not). After
    // Codenotch's HardwareNotch.
    static func notch(of screen: NSScreen) -> CGSize? {
        guard #available(macOS 12, *), let left = screen.auxiliaryTopLeftArea, let right = screen.auxiliaryTopRightArea else { return nil }
        let width = screen.frame.width - left.width - right.width
        let height = max(screen.safeAreaInsets.top, left.height, right.height)
        guard width > 0, height > 0 else { return nil }
        return CGSize(width: width, height: height)
    }

    private func watchPointer() {
        let events: NSEvent.EventTypeMask = [.mouseMoved, .leftMouseDragged]
        if let global = NSEvent.addGlobalMonitorForEvents(matching: events, handler: { [weak self] _ in
            MainActor.assumeIsolated { self?.pointerMoved() }
        }) {
            monitors.append(global)
        }
        if let local = NSEvent.addLocalMonitorForEvents(matching: events, handler: { [weak self] event in
            MainActor.assumeIsolated { self?.pointerMoved() }
            return event
        }) {
            monitors.append(local)
        }
        // Events don't come while another app tracks the mouse, or after a
        // Space change: a slow poll catches what they miss.
        let timer = Timer(timeInterval: 0.3, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.pointerMoved() }
        }
        RunLoop.main.add(timer, forMode: .common)
        poll = timer
    }

    // Where the pointer is, in the panel's top-left coordinates.
    private func pointer() -> CGPoint? {
        guard let panel else { return nil }
        let m = NSEvent.mouseLocation
        return CGPoint(x: m.x - panel.frame.minX, y: panel.frame.maxY - m.y)
    }

    // What takes the pointer while the bar is shut: a little more than the
    // sliver (or the notch), so the screen's very edge is enough to open it.
    private var hotRect: CGRect {
        let s = model.shapeFrame
        // Before the first layout there is nothing to hover.
        guard s != .zero else { return .zero }
        switch model.place {
        case .edge: return CGRect(x: s.minX - 6, y: s.minY - 10, width: s.width + 12, height: s.height + 20)
        case .notch: return CGRect(x: s.minX - 6, y: s.minY, width: s.width + 12, height: s.height + 6)
        }
    }

    func pointerMoved() {
        guard let p = pointer() else { return }
        let overShape = shapeArea.contains(p)
        let overCard = model.card && cardArea.contains(p)
        if overShape || overCard {
            closing?.cancel()
            closing = nil
            if !model.open {
                model.open = true
            }
            if overShape, !model.card, dwelling == nil {
                let work = DispatchWorkItem { [weak self] in
                    MainActor.assumeIsolated {
                        guard let self else { return }
                        self.dwelling = nil
                        guard self.model.open, let p = self.pointer(), self.shapeArea.contains(p) else { return }
                        withAnimation(Motion.glide) { self.model.card = true }
                    }
                }
                dwelling = work
                DispatchQueue.main.asyncAfter(deadline: .now() + Self.dwell, execute: work)
            }
        } else if model.open, closing == nil {
            let work = DispatchWorkItem { [weak self] in
                MainActor.assumeIsolated { self?.fold() }
            }
            closing = work
            DispatchQueue.main.asyncAfter(deadline: .now() + Self.grace, execute: work)
        }
    }

    private func fold() {
        closing = nil
        dwelling?.cancel()
        dwelling = nil
        if model.card { withAnimation(Motion.glide) { model.card = false } }
        if model.open { model.open = false }
    }

    // What keeps the bar open: the shape, and never less than what opened
    // it (or a pointer in between would open and fold it over and over).
    private var shapeArea: CGRect {
        model.open ? model.shapeFrame.insetBy(dx: -4, dy: -4).union(hotRect) : hotRect
    }

    // The card and the shadow around it.
    private var cardArea: CGRect {
        model.cardFrame == .zero ? .zero : model.cardFrame.insetBy(dx: -Layout.cardShadow, dy: -Layout.cardShadow)
    }

    private func updateHits() {
        guard let hosting else { return }
        var rects = [shapeArea]
        if model.card, cardArea != .zero { rects.append(cardArea) }
        hosting.interactiveRects = rects
    }
}

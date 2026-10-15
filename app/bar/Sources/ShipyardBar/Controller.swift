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
        let f = screen.frame
        model.notch = Self.notch(of: screen)
        model.menuBar = max(0, f.maxY - screen.visibleFrame.maxY)
        let frame: CGRect
        switch model.place {
        case .edge:
            let width = Layout.card + Layout.gap + Layout.dockWidth + 28
            let height = min(f.height - model.menuBar, 680)
            frame = CGRect(x: f.maxX - width, y: f.midY - height / 2, width: width, height: height)
        case .notch:
            let width: CGFloat = 640
            let height = min(f.height * 0.7, 560)
            frame = CGRect(x: f.midX - width / 2, y: f.maxY - height, width: width, height: height)
        }
        panel.setFrame(frame, display: true)
        updateHits()
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
        switch model.place {
        case .edge: return CGRect(x: s.minX - 6, y: s.minY - 10, width: s.width + 12, height: s.height + 20)
        case .notch: return CGRect(x: s.minX - 6, y: s.minY, width: s.width + 12, height: s.height + 6)
        }
    }

    func pointerMoved() {
        guard let p = pointer() else { return }
        let overShape = (model.open ? model.shapeFrame.insetBy(dx: -4, dy: -4) : hotRect).contains(p)
        let overCard = model.card && model.cardFrame.insetBy(dx: -6, dy: -6).contains(p)
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
                        guard self.model.open, let p = self.pointer(), self.model.shapeFrame.insetBy(dx: -4, dy: -4).contains(p) else { return }
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

    private func updateHits() {
        guard let hosting else { return }
        var rects = [model.open ? model.shapeFrame : hotRect]
        if model.card, model.cardFrame != .zero { rects.append(model.cardFrame) }
        hosting.interactiveRects = rects
    }
}

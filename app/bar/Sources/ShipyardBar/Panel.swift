import AppKit
import SwiftUI

// The panel and its views, after Codenotch's NotchPanel and NotchHostingView
// (MIT, github.com/vinzdg/codenotch).

// BarPanel floats like a status item: over other apps and the menu bar, on
// every Space and beside full-screen apps, never taking focus from the app
// being typed in.
final class BarPanel: NSPanel {
    init(frame: CGRect) {
        super.init(contentRect: frame, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        level = .statusBar
        collectionBehavior = [.canJoinAllSpaces, .stationary, .fullScreenAuxiliary, .ignoresCycle]
        isOpaque = false
        backgroundColor = .clear
        hasShadow = false
        isMovable = false
        isMovableByWindowBackground = false
        hidesOnDeactivate = false
        becomesKeyOnlyIfNeeded = true
        isReleasedWhenClosed = false
        acceptsMouseMovedEvents = true
    }

    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

// The content view holds the hosting view and nothing else, so SwiftUI's
// ideal size never sizes the panel, and it never claims a point itself.
final class BarContainerView: NSView {
    override func hitTest(_ point: NSPoint) -> NSView? {
        let local = convert(point, from: superview)
        guard bounds.contains(local) else { return nil }
        for subview in subviews.reversed() {
            if let hit = subview.hitTest(local) { return hit }
        }
        return nil
    }
}

// Most of the panel is room for the card: everywhere but the shape and the
// card lets clicks through to what is underneath.
final class BarHostingView<Content: View>: NSHostingView<Content> {
    var interactiveRects: [CGRect] = []

    override func hitTest(_ point: NSPoint) -> NSView? {
        let local = convert(point, from: superview)
        guard interactiveRects.contains(where: { $0.contains(local) }) else { return nil }
        return super.hitTest(point)
    }

    // A click on the bar acts at once, without first focusing the panel.
    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
}

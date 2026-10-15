import SwiftUI

// The bar's two shapes, after Codenotch's (MIT, github.com/vinzdg/codenotch):
// a body out of a screen edge whose ends curve into that edge, so it reads
// as part of the screen rather than a window on it. Both are Animatable in
// their own numbers, so a spring carries the shape itself and it changes
// like a body of liquid, not a rectangle being resized.

// DockShape grows out of the right edge: width out of it, height along it.
// The shoulders (concave curves where it meets the edge) and the rounded
// corners scale with the width, so the sliver it folds into has none.
struct DockShape: Shape {
    var width: CGFloat
    var height: CGFloat

    var animatableData: AnimatablePair<CGFloat, CGFloat> {
        get { AnimatablePair(width, height) }
        set {
            width = newValue.first
            height = newValue.second
        }
    }

    static func shoulder(for width: CGFloat) -> CGFloat { min(14, width * 0.26) }

    func path(in rect: CGRect) -> Path {
        let s = Self.shoulder(for: width)
        let r = min(width / 2, height / 2, 22)
        let edge = rect.maxX
        let left = edge - width
        let top = rect.midY - height / 2
        let bottom = rect.midY + height / 2
        var p = Path()
        // From the edge above the body, down the top shoulder…
        p.move(to: CGPoint(x: edge, y: top - s))
        p.addArc(center: CGPoint(x: edge - s, y: top - s), radius: s, startAngle: .degrees(0), endAngle: .degrees(90), clockwise: false)
        // …along the top, round the near corners, along the bottom…
        p.addLine(to: CGPoint(x: left + r, y: top))
        p.addArc(center: CGPoint(x: left + r, y: top + r), radius: r, startAngle: .degrees(-90), endAngle: .degrees(180), clockwise: true)
        p.addLine(to: CGPoint(x: left, y: bottom - r))
        p.addArc(center: CGPoint(x: left + r, y: bottom - r), radius: r, startAngle: .degrees(180), endAngle: .degrees(90), clockwise: true)
        p.addLine(to: CGPoint(x: edge - s, y: bottom))
        // …and up the bottom shoulder into the edge.
        p.addArc(center: CGPoint(x: edge - s, y: bottom + s), radius: s, startAngle: .degrees(-90), endAngle: .degrees(0), clockwise: false)
        p.closeSubpath()
        return p
    }
}

// NotchShape hangs from the top edge, centred: as wide and deep as the
// display's notch when shut, so it vanishes into it, and wider when open,
// with room either side of the notch.
struct NotchShape: Shape {
    var width: CGFloat
    var height: CGFloat

    var animatableData: AnimatablePair<CGFloat, CGFloat> {
        get { AnimatablePair(width, height) }
        set {
            width = newValue.first
            height = newValue.second
        }
    }

    func path(in rect: CGRect) -> Path {
        let s = min(8, height * 0.3)
        let r = min(height / 2, 12)
        let left = rect.midX - width / 2
        let right = rect.midX + width / 2
        let top = rect.minY
        let bottom = top + height
        var p = Path()
        p.move(to: CGPoint(x: left - s, y: top))
        p.addArc(center: CGPoint(x: left - s, y: top + s), radius: s, startAngle: .degrees(-90), endAngle: .degrees(0), clockwise: false)
        p.addLine(to: CGPoint(x: left, y: bottom - r))
        p.addArc(center: CGPoint(x: left + r, y: bottom - r), radius: r, startAngle: .degrees(180), endAngle: .degrees(90), clockwise: true)
        p.addLine(to: CGPoint(x: right - r, y: bottom))
        p.addArc(center: CGPoint(x: right - r, y: bottom - r), radius: r, startAngle: .degrees(90), endAngle: .degrees(0), clockwise: true)
        p.addLine(to: CGPoint(x: right, y: top + s))
        p.addArc(center: CGPoint(x: right + s, y: top + s), radius: s, startAngle: .degrees(180), endAngle: .degrees(270), clockwise: false)
        p.closeSubpath()
        return p
    }
}

// The motion, after Codenotch's NotchMotion: springs throughout, damped just
// under bouncy, so the shape settles rather than arrives.
enum Motion {
    static let unfold = Animation.spring(response: 0.62, dampingFraction: 0.72)
    static let contents = Animation.spring(response: 0.48, dampingFraction: 0.8)
    static let glide = Animation.spring(response: 0.5, dampingFraction: 0.86)
    static let reading = Animation.spring(response: 0.9, dampingFraction: 0.9)

    static func stagger(_ i: Int) -> Animation { contents.delay(min(Double(i) * 0.045, 0.18)) }
}

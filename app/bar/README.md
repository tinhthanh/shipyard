# shipyard-bar

The floating bar's native window on macOS (Settings → Appearance → Floating
bar): a sliver on the right edge of the screen, or a band around the notch,
that opens under the pointer into each login's plan limits and the agents
that need you or are working. Shipyard.app carries it as
`Contents/MacOS/shipyard-bar` and starts it while the bar is on
(`app/src-tauri/src/overlay.rs`).

It only draws. Shipyard sends it what to show and where, one JSON object a
line on stdin, and it answers clicks on stdout (see `Model.swift`); it quits
when its stdin closes.

The panel, its shapes, motion and notch measuring follow
[Codenotch](https://github.com/vinzdg/codenotch) (MIT, `LICENSE.codenotch`):
a non-activating panel that never moves while in use, with only the shapes
in it moving, on springs, and the pointer followed with a global mouse
monitor, so it opens under the pointer whatever app is in front.

```sh
swift build -c release               # .build/release/shipyard-bar
swift test                           # BAR_SNAPSHOTS=dir also renders its states to PNGs
.build/release/shipyard-bar --demo --place edge   # on its own, with made-up readings
```

`make app-build` builds it for the app's architectures and bundles it
(`tauri.bar.conf.json`).

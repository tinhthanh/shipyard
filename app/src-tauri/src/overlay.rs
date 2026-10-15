use serde::{Deserialize, Serialize};
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager, State};

// The floating bar (Settings → Appearance): on macOS a native helper,
// shipyard-bar (app/bar, SwiftUI, after Codenotch's notch; MIT), which the
// app carries beside its own executable. Shipyard starts it while the bar is
// on and speaks to it in JSON lines: where it sits and what it shows go in
// on its stdin (the main window works them out, src/overlay), and what is
// clicked on it comes back on its stdout, for the main window to open. The
// bar quits when its stdin closes, so it goes with Shipyard however Shipyard
// ends.

#[derive(Deserialize, Serialize, Clone, Copy, PartialEq, Debug)]
#[serde(rename_all = "lowercase")]
pub enum Place {
    Edge,
    Notch,
}

#[derive(Deserialize, Serialize, Clone, Debug)]
pub struct Target {
    #[serde(rename = "box")]
    box_name: String,
    session: String,
}

// The running bar, if any.
#[derive(Default)]
pub struct Bar(Mutex<Option<Running>>);

pub struct Running {
    child: std::process::Child,
    stdin: std::process::ChildStdin,
}

impl Running {
    fn send(&mut self, line: &serde_json::Value) -> Result<(), String> {
        use std::io::Write;
        let mut text = line.to_string();
        text.push('\n');
        self.stdin.write_all(text.as_bytes()).and_then(|_| self.stdin.flush()).map_err(|e| format!("the floating bar stopped: {e}"))
    }

    fn alive(&mut self) -> bool {
        matches!(self.child.try_wait(), Ok(None))
    }
}

impl Drop for Running {
    fn drop(&mut self) {
        let _ = self.child.kill();
        let _ = self.child.wait();
    }
}

// overlay_show starts the bar at place, or moves it there.
#[tauri::command]
pub fn overlay_show(app: AppHandle, bar: State<Bar>, place: Place) -> Result<(), String> {
    let mut slot = bar.0.lock().map_err(|e| e.to_string())?;
    if let Some(running) = slot.as_mut() {
        if running.alive() {
            return running.send(&serde_json::json!({ "type": "place", "place": place }));
        }
    }
    *slot = Some(start(&app, place)?);
    Ok(())
}

// overlay_hide stops the bar.
#[tauri::command]
pub fn overlay_hide(bar: State<Bar>) {
    if let Ok(mut slot) = bar.0.lock() {
        slot.take();
    }
}

// overlay_state is what the bar shows (src/overlay/bar-state.ts): logins
// and their plan windows, and the agents that need you or are working.
#[tauri::command]
pub fn overlay_state(bar: State<Bar>, state: serde_json::Value) -> Result<(), String> {
    let mut slot = bar.0.lock().map_err(|e| e.to_string())?;
    let Some(running) = slot.as_mut() else { return Ok(()) };
    let mut line = state;
    if let Some(o) = line.as_object_mut() {
        o.insert("type".into(), "state".into());
    }
    running.send(&line)
}

// The bar goes with the main window: closing that still quits Shipyard.
pub fn on_main_destroyed(app: &AppHandle) {
    if let Some(bar) = app.try_state::<Bar>() {
        overlay_hide(bar);
    }
}

#[cfg(target_os = "macos")]
fn start(app: &AppHandle, place: Place) -> Result<Running, String> {
    use std::io::BufRead;
    use std::process::{Command, Stdio};
    let path = helper()?;
    let place_arg = match place {
        Place::Edge => "edge",
        Place::Notch => "notch",
    };
    let mut child = Command::new(&path)
        .args(["--place", place_arg])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|e| format!("couldn't start the floating bar ({}): {e}", path.display()))?;
    let stdin = child.stdin.take().ok_or("the floating bar has no stdin")?;
    let stdout = child.stdout.take().ok_or("the floating bar has no stdout")?;
    let handle = app.clone();
    std::thread::spawn(move || {
        for line in std::io::BufReader::new(stdout).lines().map_while(Result::ok) {
            let Ok(msg) = serde_json::from_str::<serde_json::Value>(&line) else { continue };
            if msg.get("type").and_then(|t| t.as_str()) == Some("open") {
                let target = serde_json::from_value::<Target>(msg.clone()).ok();
                let app = handle.clone();
                let _ = handle.run_on_main_thread(move || open_main(&app, target));
            }
        }
    });
    Ok(Running { child, stdin })
}

#[cfg(not(target_os = "macos"))]
fn start(_app: &AppHandle, _place: Place) -> Result<Running, String> {
    Err("the floating bar is for macOS".into())
}

// The helper beside the app's executable (Contents/MacOS/shipyard-bar); in
// a development build, the one swift build made in app/bar.
#[cfg(target_os = "macos")]
fn helper() -> Result<std::path::PathBuf, String> {
    let exe = std::env::current_exe().map_err(|e| e.to_string())?;
    let beside = exe.with_file_name("shipyard-bar");
    if beside.exists() {
        return Ok(beside);
    }
    #[cfg(debug_assertions)]
    {
        let dev = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../bar/.build/release/shipyard-bar");
        if dev.exists() {
            return Ok(dev);
        }
    }
    Err("this Shipyard has no floating bar (shipyard-bar is missing); in development, run swift build -c release in app/bar".into())
}

// open_main brings the main window forward and has it open what was
// clicked on the bar: a session, or Home when there is none.
fn open_main(app: &AppHandle, target: Option<Target>) {
    let Some(main) = app.get_webview_window("main") else { return };
    let _ = main.unminimize();
    let _ = main.show();
    let _ = main.set_focus();
    let _ = main.emit_to("main", "berth://overlay-open", target);
}

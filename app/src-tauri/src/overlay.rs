use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

// The floating bar (Settings → Appearance): a small window that stays over
// every app, every Space and full-screen apps, with each login's plan limits
// and the agents that need you or are working. Its page is the app's own
// (index.html?window=overlay, app/src/overlay), which sizes the window to
// what it shows and asks the main window to open what was clicked.
//
// It sits on the right edge of the screen with the menu bar, or around the
// display's notch, after Codenotch (MIT), whose notch it imitates.

const LABEL: &str = "overlay";
// How far the edge bar keeps from the screen's edge: none, it grows out of it.
const EDGE_GAP: f64 = 0.0;

#[derive(Deserialize, Serialize, Clone, Copy, PartialEq, Debug)]
#[serde(rename_all = "lowercase")]
pub enum Place {
    Edge,
    Notch,
}

impl Place {
    fn as_str(self) -> &'static str {
        match self {
            Place::Edge => "edge",
            Place::Notch => "notch",
        }
    }
}

// The screen the bar is on, in points: the one with the menu bar.
#[derive(Serialize, Default)]
pub struct Screen {
    width: f64,
    height: f64,
    // The display's own notch, when it has one.
    notch: Option<Notch>,
    // The menu bar's height (or the notch's, which is deeper).
    top: f64,
}

#[derive(Serialize)]
struct Notch {
    width: f64,
    height: f64,
}

// overlay_show opens the bar at place, or moves it there. It stays hidden
// until its page has measured itself (overlay_fit). Async: a window built
// from a command on the main thread would wait on that thread.
#[tauri::command]
pub async fn overlay_show(app: AppHandle, place: Place) -> Result<(), String> {
    let query = format!("window=overlay&place={}", place.as_str());
    // Moved: the same window, on the page for its new place. (Closing it to
    // make another would race the label, which a close frees only later.)
    if let Some(w) = app.get_webview_window(LABEL) {
        let mut url = w.url().map_err(|e| e.to_string())?;
        if url.query() == Some(query.as_str()) {
            return Ok(());
        }
        url.set_query(Some(&query));
        w.hide().map_err(|e| e.to_string())?;
        w.navigate(url).map_err(|e| e.to_string())?;
        #[cfg(target_os = "macos")]
        {
            let floating = w.clone();
            w.run_on_main_thread(move || float(&floating, place)).map_err(|e| e.to_string())?;
        }
        return Ok(());
    }
    let url = WebviewUrl::App(format!("index.html?{query}").into());
    let w = WebviewWindowBuilder::new(&app, LABEL, url)
        .title("Shipyard bar")
        .decorations(false)
        .transparent(true)
        .shadow(false)
        .resizable(false)
        .always_on_top(true)
        .visible_on_all_workspaces(true)
        .skip_taskbar(true)
        .focused(false)
        .visible(false)
        .inner_size(40.0, 40.0)
        .build()
        .map_err(|e| e.to_string())?;
    #[cfg(target_os = "macos")]
    {
        let floating = w.clone();
        w.run_on_main_thread(move || float(&floating, place)).map_err(|e| e.to_string())?;
    }
    Ok(())
}

// overlay_hide closes the bar.
#[tauri::command]
pub fn overlay_hide(app: AppHandle) {
    if let Some(w) = app.get_webview_window(LABEL) {
        let _ = w.close();
    }
}

// overlay_screen is what the bar's page lays itself out around.
#[tauri::command]
pub fn overlay_screen(app: AppHandle) -> Screen {
    screen(&app)
}

// overlay_fit sizes the bar to its page and places it: on the edge, centred
// top to bottom and growing to the left; at the notch, centred under the
// top of the screen and growing down.
#[tauri::command]
pub fn overlay_fit(app: AppHandle, window: WebviewWindow, width: f64, height: f64, place: Place) -> Result<(), String> {
    if window.label() != LABEL {
        return Err("only the floating bar fits itself".into());
    }
    let s = screen(&app);
    let (ox, oy) = origin(&app);
    let (x, y) = match place {
        Place::Edge => (s.width - width - EDGE_GAP, ((s.height - height) / 2.0).max(s.top)),
        Place::Notch => ((s.width - width) / 2.0, 0.0),
    };
    window.set_size(LogicalSize::new(width, height)).map_err(|e| e.to_string())?;
    window.set_position(LogicalPosition::new(ox + x, oy + y)).map_err(|e| e.to_string())?;
    #[cfg(target_os = "macos")]
    front(&window);
    #[cfg(not(target_os = "macos"))]
    window.show().map_err(|e| e.to_string())?;
    Ok(())
}

#[derive(Deserialize, Serialize, Clone)]
pub struct Target {
    #[serde(rename = "box")]
    box_name: String,
    session: String,
}

// overlay_open brings the main window forward and has it open what was
// clicked on the bar: a session, or Home when there is none.
#[tauri::command]
pub fn overlay_open(app: AppHandle, target: Option<Target>) -> Result<(), String> {
    let main = app.get_webview_window("main").ok_or("the main window is gone")?;
    let _ = main.unminimize();
    let _ = main.show();
    let _ = main.set_focus();
    main.emit_to("main", "berth://overlay-open", target).map_err(|e| e.to_string())
}

// The bar goes with the main window: closing that still quits Shipyard.
pub fn on_main_destroyed(app: &AppHandle) {
    overlay_hide(app.clone());
}

fn origin(app: &AppHandle) -> (f64, f64) {
    match app.primary_monitor() {
        Ok(Some(m)) => {
            let p = m.position().to_logical::<f64>(m.scale_factor());
            (p.x, p.y)
        }
        _ => (0.0, 0.0),
    }
}

#[cfg(not(target_os = "macos"))]
fn screen(app: &AppHandle) -> Screen {
    match app.primary_monitor() {
        Ok(Some(m)) => {
            let size = m.size().to_logical::<f64>(m.scale_factor());
            Screen { width: size.width, height: size.height, notch: None, top: 0.0 }
        }
        _ => Screen::default(),
    }
}

#[cfg(target_os = "macos")]
mod mac {
    use objc2::encode::{Encode, Encoding};

    #[repr(C)]
    #[derive(Clone, Copy, Default)]
    pub struct Point {
        pub x: f64,
        pub y: f64,
    }
    #[repr(C)]
    #[derive(Clone, Copy, Default)]
    pub struct Size {
        pub width: f64,
        pub height: f64,
    }
    #[repr(C)]
    #[derive(Clone, Copy, Default)]
    pub struct Rect {
        pub origin: Point,
        pub size: Size,
    }
    #[repr(C)]
    #[derive(Clone, Copy, Default)]
    pub struct Insets {
        pub top: f64,
        pub left: f64,
        pub bottom: f64,
        pub right: f64,
    }
    unsafe impl Encode for Point {
        const ENCODING: Encoding = Encoding::Struct("CGPoint", &[f64::ENCODING, f64::ENCODING]);
    }
    unsafe impl Encode for Size {
        const ENCODING: Encoding = Encoding::Struct("CGSize", &[f64::ENCODING, f64::ENCODING]);
    }
    unsafe impl Encode for Rect {
        const ENCODING: Encoding = Encoding::Struct("CGRect", &[Point::ENCODING, Size::ENCODING]);
    }
    unsafe impl Encode for Insets {
        const ENCODING: Encoding = Encoding::Struct("NSEdgeInsets", &[f64::ENCODING, f64::ENCODING, f64::ENCODING, f64::ENCODING]);
    }
}

// The screen with the menu bar (the first NSScreen), its notch measured from
// the menu bar strips either side of it, which AppKit describes directly.
// The deepest of those and the safe area is the notch's depth: the safe area
// collapses when the menu bar hides, the hole does not (after Codenotch).
#[cfg(target_os = "macos")]
fn screen(_app: &AppHandle) -> Screen {
    use mac::{Insets, Rect};
    use objc2::msg_send;
    use objc2::runtime::{AnyClass, AnyObject};
    unsafe {
        let Some(class) = AnyClass::get(c"NSScreen") else { return Screen::default() };
        let screens: *mut AnyObject = msg_send![class, screens];
        let Some(screens) = screens.as_ref() else { return Screen::default() };
        let count: usize = msg_send![screens, count];
        if count == 0 {
            return Screen::default();
        }
        let first: *mut AnyObject = msg_send![screens, objectAtIndex: 0usize];
        let Some(first) = first.as_ref() else { return Screen::default() };
        let frame: Rect = msg_send![first, frame];
        let visible: Rect = msg_send![first, visibleFrame];
        // Above the visible frame: the menu bar (zero when it hides).
        let menu = (frame.size.height - (visible.origin.y - frame.origin.y) - visible.size.height).max(0.0);
        let mut notch = None;
        let mut top = menu;
        let has_aux: bool = msg_send![first, respondsToSelector: objc2::sel!(auxiliaryTopLeftArea)];
        if has_aux {
            let left: Rect = msg_send![first, auxiliaryTopLeftArea];
            let right: Rect = msg_send![first, auxiliaryTopRightArea];
            let insets: Insets = msg_send![first, safeAreaInsets];
            let width = frame.size.width - left.size.width - right.size.width;
            let height = insets.top.max(left.size.height).max(right.size.height);
            if left.size.width > 0.0 && right.size.width > 0.0 && width > 0.0 && height > 0.0 {
                notch = Some(Notch { width, height });
                top = top.max(height);
            }
        }
        Screen { width: frame.size.width, height: frame.size.height, notch, top }
    }
}

// float makes the bar float like a status item: over other apps' windows and
// the menu bar, on every Space and beside full-screen apps, and staying when
// Shipyard is not the active app.
#[cfg(target_os = "macos")]
fn float(w: &WebviewWindow, place: Place) {
    use objc2::msg_send;
    use objc2::runtime::AnyObject;
    // NSStatusWindowLevel is the menu bar's own items; the notch bar covers
    // the menu bar, so one above.
    let level: isize = match place {
        Place::Edge => 25,
        Place::Notch => 26,
    };
    // canJoinAllSpaces | stationary | ignoresCycle | fullScreenAuxiliary
    let behavior: usize = (1 << 0) | (1 << 4) | (1 << 6) | (1 << 8);
    let Ok(ns) = w.ns_window() else { return };
    unsafe {
        let Some(ns) = (ns as *mut AnyObject).as_ref() else { return };
        let _: () = msg_send![ns, setLevel: level];
        let _: () = msg_send![ns, setCollectionBehavior: behavior];
        let _: () = msg_send![ns, setHidesOnDeactivate: false];
        let _: () = msg_send![ns, setMovable: false];
        let _: () = msg_send![ns, setExcludedFromWindowsMenu: true];
    }
}

// front shows the bar without making Shipyard the active app.
#[cfg(target_os = "macos")]
fn front(w: &WebviewWindow) {
    use objc2::msg_send;
    use objc2::runtime::AnyObject;
    let Ok(ns) = w.ns_window() else { return };
    unsafe {
        if let Some(ns) = (ns as *mut AnyObject).as_ref() {
            let _: () = msg_send![ns, orderFrontRegardless];
        }
    }
}

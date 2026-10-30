// What the app does differently on Windows, in one place: the rest of the
// app calls these under #[cfg(windows)], one line each.

use std::path::PathBuf;
use std::process::Command;

use ::windows::Win32::UI::Input::KeyboardAndMouse::{
    GetKeyState, VIRTUAL_KEY, VK_CONTROL, VK_MENU, VK_SHIFT,
};
use tauri::{App, AppHandle, Runtime, Webview};
use webview2_com::AcceleratorKeyPressedEventHandler;
use webview2_com::Microsoft::Web::WebView2::Win32::{
    COREWEBVIEW2_KEY_EVENT_KIND, COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN,
    COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN,
};

use crate::menu::{self, Keys};

// no_window keeps a console program the app runs (berth) from opening a
// console window each time.
pub fn no_window(cmd: &mut Command) {
    use std::os::windows::process::CommandExt;
    const CREATE_NO_WINDOW: u32 = 0x0800_0000;
    cmd.creation_flags(CREATE_NO_WINDOW);
}

// token_path is the agent's token in the local AppData, where berth keeps its
// state on Windows (statefile.Home), unless the state hasn't moved there from
// the roaming AppData (token) yet.
pub fn token_path(token: PathBuf) -> PathBuf {
    if let Some(local) = dirs::data_local_dir() {
        let moved = local.join("berth").join("client").join("ui-token");
        if moved.exists() || !token.exists() {
            return moved;
        }
    }
    token
}

// open_terminal opens Windows Terminal where it is installed (Windows 11 has
// it), else a console of its own.
pub fn open_terminal() -> Result<(), String> {
    use std::os::windows::process::CommandExt;
    const CREATE_NEW_CONSOLE: u32 = 0x0000_0010;
    Command::new("wt.exe")
        .spawn()
        .or_else(|_| {
            Command::new("cmd.exe")
                .creation_flags(CREATE_NEW_CONSOLE)
                .spawn()
        })
        .map(|_| ())
        .map_err(|e| format!("couldn't open a terminal: {e}"))
}

// setup: the installer registers berth://; a build run from the repository
// registers it itself. Windows has no menu either: the webview handles the
// keys, as on Linux.
pub fn setup<R: Runtime>(app: &mut App<R>) {
    #[cfg(debug_assertions)]
    {
        use tauri_plugin_deep_link::DeepLinkExt;
        let _ = app.deep_link().register_all();
    }
    let _ = app;
}

// On Windows a browser pane is a WebView2 of its own, and nothing like the
// Mac's menu bar sees a key before it: the app's shortcuts went to the page.
// The pane's controller hears each key pressed with a modifier before the
// page does (AcceleratorKeyPressed). One of the app's shortcuts goes to the
// app as its menu item would (menu.rs), and the page never sees it. A page
// can't send one: only a key really pressed reaches the controller.

fn held(key: VIRTUAL_KEY) -> bool {
    unsafe { GetKeyState(key.0 as i32) < 0 }
}

pub fn forward_shortcuts<R: Runtime>(app: AppHandle<R>, wv: &Webview<R>) -> Result<(), String> {
    let shortcuts = menu::accelerators();
    wv.with_webview(move |pv| {
        let handler = AcceleratorKeyPressedEventHandler::create(Box::new(move |_, args| {
            let Some(args) = args else { return Ok(()) };
            let mut kind = COREWEBVIEW2_KEY_EVENT_KIND::default();
            let mut vk = 0u32;
            unsafe {
                args.KeyEventKind(&mut kind)?;
                args.VirtualKey(&mut vk)?;
            }
            if kind != COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN
                && kind != COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN
            {
                return Ok(());
            }
            let keys = Keys {
                ctrl: held(VK_CONTROL),
                shift: held(VK_SHIFT),
                alt: held(VK_MENU),
                vk,
            };
            if let Some((_, id)) = shortcuts.iter().find(|(k, _)| *k == keys) {
                unsafe { args.SetHandled(true)? };
                menu::on_event(&app, id);
            }
            Ok(())
        }));
        let mut token = 0i64;
        if let Err(e) = unsafe {
            pv.controller()
                .add_AcceleratorKeyPressed(&handler, &mut token)
        } {
            eprintln!("berth: a browser pane's shortcuts stay the page's: {e}");
        }
    })
    .map_err(|e| e.to_string())
}

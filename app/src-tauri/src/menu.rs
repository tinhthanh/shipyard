// The menu bar. Its File, View, Go and Help items are the app's shortcuts,
// from the same table the webview reads (app/src/lib/shortcuts.json). A menu
// item's accelerator is matched by macOS before it can take the key for
// itself (⌘. would be "cancel", ⌘⇧/ the Help search), and it still works
// while a browser pane, a child webview, has focus. Each item sends its id
// as berth://menu; the webview runs it through the same handler as the key
// (hooks/use-shortcuts.ts).
//
// The table leaves ⌘C, ⌘V, ⌘A and ⌘F alone: Edit keeps the system's items,
// and terminals and text fields need them.

use serde::Deserialize;
use tauri::menu::{
    AboutMetadata, Menu, MenuItem, PredefinedMenuItem, Submenu, HELP_SUBMENU_ID, WINDOW_SUBMENU_ID,
};
use tauri::{AppHandle, Emitter, Manager, Runtime};

const TABLE: &str = include_str!("../../src/lib/shortcuts.json");

#[derive(Deserialize)]
struct Table {
    shortcuts: Vec<Shortcut>,
}

#[derive(Deserialize)]
struct Shortcut {
    id: String,
    group: String,
    label: String,
    accel: Option<String>,
    count: Option<u32>,
    #[serde(default)]
    sep: bool,
}

// The one item that is the window's, not the webview's: Close window. Its
// key, ⌘⇧W, is File → Close group's (the table's close-group), which the
// webview runs: it closes the tab group in front, or with only one, the
// window. ⌘W closes the focused pane.
const CLOSE_WINDOW: &str = "close-window";

fn table() -> Table {
    serde_json::from_str(TABLE).expect("app/src/lib/shortcuts.json is not valid")
}

// fill appends the table's items for a menu to sub.
fn fill<R: Runtime>(
    app: &AppHandle<R>,
    sub: &Submenu<R>,
    table: &Table,
    group: &str,
) -> tauri::Result<()> {
    let mut first = true;
    for s in table.shortcuts.iter().filter(|s| s.group == group) {
        let Some(accel) = s.accel.as_deref() else {
            continue;
        };
        if s.sep && !first {
            sub.append(&PredefinedMenuItem::separator(app)?)?;
        }
        first = false;
        match s.count {
            Some(n) => {
                for i in 1..=n {
                    let n = i.to_string();
                    let item = MenuItem::with_id(
                        app,
                        format!("{}-{n}", s.id),
                        s.label.replace("{n}", &n),
                        true,
                        Some(accel.replace("{n}", &n)),
                    )?;
                    sub.append(&item)?;
                }
            }
            None => sub.append(&MenuItem::with_id(
                app,
                s.id.as_str(),
                &s.label,
                true,
                Some(accel),
            )?)?,
        }
    }
    Ok(())
}

// Only the Mac app has a menu bar (lib.rs).
#[cfg_attr(not(target_os = "macos"), allow(dead_code))]
pub fn build<R: Runtime>(app: &AppHandle<R>) -> tauri::Result<Menu<R>> {
    let table = table();
    // "Shipyard", as the window and the Dock name it, not the crate's "berth".
    let name = app
        .config()
        .product_name
        .clone()
        .unwrap_or_else(|| app.package_info().name.clone());
    let about = AboutMetadata {
        name: Some(name.clone()),
        version: Some(app.package_info().version.to_string()),
        ..Default::default()
    };
    let menu = Menu::new(app)?;

    #[cfg(target_os = "macos")]
    menu.append(&Submenu::with_items(
        app,
        &name,
        true,
        &[
            &PredefinedMenuItem::about(app, Some(&format!("About {name}")), Some(about.clone()))?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::services(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::hide(app, Some(&format!("Hide {name}")))?,
            &PredefinedMenuItem::hide_others(app, None)?,
            &PredefinedMenuItem::show_all(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::quit(app, Some(&format!("Quit {name}")))?,
        ],
    )?)?;

    let file = Submenu::new(app, "File", true)?;
    fill(app, &file, &table, "File")?;
    #[cfg(not(target_os = "macos"))]
    {
        file.append(&PredefinedMenuItem::separator(app)?)?;
        file.append(&PredefinedMenuItem::quit(app, None)?)?;
    }
    menu.append(&file)?;

    menu.append(&Submenu::with_items(
        app,
        "Edit",
        true,
        &[
            &PredefinedMenuItem::undo(app, None)?,
            &PredefinedMenuItem::redo(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &PredefinedMenuItem::cut(app, None)?,
            &PredefinedMenuItem::copy(app, None)?,
            &PredefinedMenuItem::paste(app, None)?,
            &PredefinedMenuItem::select_all(app, None)?,
        ],
    )?)?;

    let view = Submenu::new(app, "View", true)?;
    fill(app, &view, &table, "View")?;
    #[cfg(target_os = "macos")]
    {
        view.append(&PredefinedMenuItem::separator(app)?)?;
        view.append(&PredefinedMenuItem::fullscreen(app, None)?)?;
    }
    menu.append(&view)?;

    let go = Submenu::new(app, "Go", true)?;
    fill(app, &go, &table, "Go")?;
    menu.append(&go)?;

    // The Window and Help ids make them macOS's Window menu (with the open
    // windows listed) and Help menu (with its search field).
    menu.append(&Submenu::with_id_and_items(
        app,
        WINDOW_SUBMENU_ID,
        "Window",
        true,
        &[
            &PredefinedMenuItem::minimize(app, None)?,
            &PredefinedMenuItem::maximize(app, None)?,
            &PredefinedMenuItem::separator(app)?,
            &MenuItem::with_id(app, CLOSE_WINDOW, "Close window", true, None::<&str>)?,
        ],
    )?)?;

    let help = Submenu::with_id(app, HELP_SUBMENU_ID, "Help", true)?;
    fill(app, &help, &table, "Help")?;
    #[cfg(not(target_os = "macos"))]
    help.append(&PredefinedMenuItem::about(app, None, Some(about))?)?;
    #[cfg(target_os = "macos")]
    let _ = about;
    menu.append(&help)?;

    Ok(menu)
}

// on_event sends a menu item's id to the webview, or closes the window.
pub fn on_event<R: Runtime>(app: &AppHandle<R>, id: &str) {
    if id == CLOSE_WINDOW {
        if let Some(w) = app.get_webview_window("main") {
            let _ = w.close();
        }
        return;
    }
    let _ = app.emit("berth://menu", id);
}

// Keys is a key with the modifiers held, as a browser pane on Windows hears
// it (keys_windows in browser.rs): vk is the Windows virtual-key code.
#[cfg_attr(not(windows), allow(dead_code))]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Keys {
    pub ctrl: bool,
    pub shift: bool,
    pub alt: bool,
    pub vk: u32,
}

// virtual_key is a key name from an accelerator as a Windows virtual-key
// code, for the keys the table uses.
#[cfg_attr(not(windows), allow(dead_code))]
fn virtual_key(name: &str) -> Option<u32> {
    let one = |c: char| name.len() == 1 && name.starts_with(c);
    Some(match name {
        n if n.len() == 1 && n.as_bytes()[0].is_ascii_alphanumeric() => n.to_ascii_uppercase().as_bytes()[0] as u32,
        _ if one('-') => 0xBD,
        _ if one('=') => 0xBB,
        _ if one('.') => 0xBE,
        _ if one('/') => 0xBF,
        _ if one('[') => 0xDB,
        _ if one(']') => 0xDD,
        _ if one('\\') => 0xDC,
        "Tab" => 0x09,
        _ => return None,
    })
}

#[cfg_attr(not(windows), allow(dead_code))]
fn parse_keys(accel: &str) -> Option<Keys> {
    let mut k = Keys { ctrl: false, shift: false, alt: false, vk: 0 };
    let parts: Vec<&str> = accel.split('+').collect();
    // A key that is "+" itself isn't in the table; "CmdOrCtrl+-" splits fine.
    let (key, mods) = parts.split_last()?;
    for m in mods {
        match *m {
            "CmdOrCtrl" | "Ctrl" | "Control" => k.ctrl = true,
            "Shift" => k.shift = true,
            "Alt" | "Option" => k.alt = true,
            _ => return None,
        }
    }
    k.vk = virtual_key(key)?;
    Some(k)
}

// accelerators is the table's shortcuts that have an accelerator, as keys
// and the menu id each one sends (with -n for a counted one, as the menu).
#[cfg_attr(not(windows), allow(dead_code))]
pub fn accelerators() -> Vec<(Keys, String)> {
    let mut out = Vec::new();
    for s in table().shortcuts {
        let Some(accel) = s.accel.as_deref() else { continue };
        match s.count {
            Some(n) => {
                for i in 1..=n {
                    if let Some(k) = parse_keys(&accel.replace("{n}", &i.to_string())) {
                        out.push((k, format!("{}-{i}", s.id)));
                    }
                }
            }
            None => {
                if let Some(k) = parse_keys(accel) {
                    out.push((k, s.id.clone()));
                }
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_accelerator_has_keys_a_pane_on_windows_can_match() {
        let keys = accelerators();
        let with_accel: usize = table()
            .shortcuts
            .iter()
            .filter(|s| s.accel.is_some())
            .map(|s| s.count.unwrap_or(1) as usize)
            .sum();
        assert_eq!(keys.len(), with_accel, "an accelerator whose key isn't known: add it to virtual_key");
        let ctrl = |vk| Keys { ctrl: true, shift: false, alt: false, vk };
        assert!(keys.contains(&(ctrl(b'N' as u32), "new-worktree".to_string())));
        assert!(keys.contains(&(Keys { ctrl: true, shift: true, alt: false, vk: b'B' as u32 }, "new-browser".to_string())));
        assert!(keys.iter().any(|(k, id)| *k == ctrl(0xDB) && id == "back"));
        assert!(keys.iter().any(|(k, _)| *k == ctrl(b'3' as u32)));
    }

    // Every shortcut in the table parses, sits in a menu the bar has, and
    // never takes a key that terminals and text need.
    #[test]
    fn table_is_sound() {
        let t = table();
        assert!(!t.shortcuts.is_empty());
        for s in &t.shortcuts {
            assert!(
                ["File", "View", "Go", "Help"].contains(&s.group.as_str()),
                "{}: unknown group {}",
                s.id,
                s.group
            );
            if let Some(a) = &s.accel {
                for taken in [
                    "CmdOrCtrl+C",
                    "CmdOrCtrl+V",
                    "CmdOrCtrl+A",
                    "CmdOrCtrl+F",
                    "CmdOrCtrl+Q",
                    "CmdOrCtrl+H",
                    "CmdOrCtrl+M",
                ] {
                    assert_ne!(a, taken, "{} takes {}", s.id, taken);
                }
                assert_eq!(
                    s.count.is_some(),
                    a.contains("{n}"),
                    "{}: count and {{n}} go together",
                    s.id
                );
            }
        }
    }
}

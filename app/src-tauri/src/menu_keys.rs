// The menu's shortcuts as Windows keys, for a browser pane on Windows
// (forward_shortcuts in windows.rs), which hears keys before any menu could.
// A child of menu.rs (menu.rs declares it), so it reads the table as menu.rs
// does.

// Keys is a key with the modifiers held, as a browser pane on Windows hears
// it (forward_shortcuts in windows.rs): vk is the Windows virtual-key code.
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
    for s in super::table().shortcuts {
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
        let with_accel: usize = super::super::table()
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
}

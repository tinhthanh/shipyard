// Which desktop app this is. The Mac app is the main one; the Linux app is
// an alpha (WebKitGTK instead of WKWebView, no menu bar, Ctrl for ⌘). A
// page in a browser keeps the Mac app's look and keys wherever it runs (the
// e2e suite runs on Linux); ?platform=linux shows the Linux app's instead,
// for screenshots and tests.

import { ISSUES } from "../platform/brand.ts";

export { APP_NAME } from "../platform/brand.ts";

const forced = (() => {
  try {
    return new URLSearchParams(location.search).get("platform");
  } catch {
    return null;
  }
})();

const tauri = typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
const nav = typeof navigator === "undefined" ? "" : `${navigator.platform} ${navigator.userAgent}`;

export const IS_WINDOWS = forced ? forced === "windows" : tauri && /Windows|Win32|Win64/.test(nav);
// The Windows app (a fork's alpha) takes the Linux app's way wherever that
// is the way of a desktop that isn't a Mac: Ctrl for ⌘, no menu bar, no
// floating bar, "this computer". So IS_LINUX is true there too.
export const IS_LINUX = forced ? forced === "linux" || forced === "windows" : tauri && (/Linux/.test(nav) || IS_WINDOWS);
export const IS_MAC = !IS_LINUX;

// The Linux app is an alpha: it says so beside its name.
export const LINUX_ALPHA = IS_LINUX;

export const LINUX_BUGS = `${ISSUES}/new?labels=linux&title=Linux%3A+`;

// How the Linux app's keys meet a terminal's (hooks/use-shortcuts.ts).
export const LINUX_TERMINAL_KEYS =
  "Ctrl stands in for ⌘. In a terminal, Ctrl and a letter go to the shell (Ctrl+W, Ctrl+D, Ctrl+K…); add Shift for the app's (Ctrl+Shift+T, Ctrl+Shift+K). Ctrl+Shift+C and Ctrl+Shift+V copy and paste there.";

// thisComputer names the laptop the way its owner would: "this Mac", or
// "this computer" in the Linux app.
export const thisComputer = (s: string): string => (IS_LINUX ? s.replaceAll("this Mac", "this computer").replaceAll("This Mac", "This computer") : s);

// The key that stands in for ⌘: ⌘ on a Mac, Ctrl everywhere else.
export const modKey = (e: { metaKey: boolean; ctrlKey: boolean }): boolean => (IS_MAC ? e.metaKey : e.ctrlKey);

const NAMES: Record<string, string> = { "⌃": "Ctrl", "⌘": "Ctrl", "⌥": "Alt", "⇧": "Shift" };
const ORDER = ["Ctrl", "Alt", "Shift"];
const KEYS: Record<string, string> = { "↵": "Enter", "⇥": "Tab", "⌫": "Backspace" };

// platformKeys writes a key hint for this platform: as it is on a Mac
// ("⌘⇧D"), and with Ctrl, Alt and Shift elsewhere ("Ctrl+Shift+D"), the
// keys the Linux app takes (hooks/use-shortcuts.ts).
export function platformKeys(s: string, mac = IS_MAC): string {
  if (mac || !/[⌃⌘⌥⇧]/.test(s)) return s;
  return s
    .replace(/([⌃⌘⌥⇧]+)-click/g, (_, m: string) => `${names(m).join("+")}-click`)
    .replace(/([⌃⌘⌥⇧]+) ?([^\s⌃⌘⌥⇧]+)/g, (_, m: string, key: string) => [...names(m), KEYS[key] ?? key].join("+"));
}

function names(mods: string): string[] {
  const out = new Set([...mods].map((c) => NAMES[c]).filter(Boolean));
  return ORDER.filter((n) => out.has(n));
}

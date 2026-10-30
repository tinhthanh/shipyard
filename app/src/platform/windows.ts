// The Windows app's own ways, in one place. Upstream files call these
// through one-line seams; each is a no-op on a Mac and on Linux.

import { useEffect } from "react";

import type { Theme } from "@/lib/api";
import { IS_WINDOWS } from "@/lib/platform";

// The command-line link (Settings → General) needs a symlink and
// ~/.local/bin, neither of which Windows has.
export const HAS_CLI_LINK = !IS_WINDOWS;

// What Start at login does here, when that isn't the Mac's way: Windows
// starts the agent at sign-in, and nothing restarts it.
export const START_AT_LOGIN_NOTE: string | undefined = IS_WINDOWS ? "Also starts it whenever you sign in to Windows." : undefined;

// blockReloadKeys: WebView2 reloads the page on F5 and Ctrl+R, which drops
// the window's state (open tabs, drafts) for nothing. A dev build keeps them,
// to reload code.
export function blockReloadKeys() {
  if (!IS_WINDOWS || import.meta.env.DEV) return;
  window.addEventListener(
    "keydown",
    (e) => {
      if (e.key === "F5" || (e.ctrlKey && !e.altKey && e.key.toLowerCase() === "r")) e.preventDefault();
    },
    true,
  );
}

// useTitleBarTheme: Windows draws the title bar itself, light or dark by the
// window's theme. null (following the system) leaves it unset, so the page
// keeps reading the system's appearance and not the one set here.
export function useTitleBarTheme(appearance: Theme["appearance"] | null) {
  useEffect(() => {
    if (!IS_WINDOWS) return;
    import("@tauri-apps/api/window").then(({ getCurrentWindow }) => getCurrentWindow().setTheme(appearance)).catch(() => {});
  }, [appearance]);
}

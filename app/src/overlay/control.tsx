import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { lazy, Suspense, useEffect } from "react";

import { isTauri } from "@/lib/api";
import { IS_LINUX } from "@/lib/platform";
import { usePrefs } from "@/lib/prefs";
import { focusSession, goHome } from "@/lib/workspaces";

// What a click on the floating bar asks the main window to open: a session,
// or Home (null).
export interface OverlayTarget {
  box: string;
  session: string;
}

export const OVERLAY_OPEN = "berth://overlay-open";

// useOverlayControl runs in the main window: it starts, moves and stops the
// floating bar as Settings › Appearance says (overlay.rs runs the native
// helper, app/bar), and opens here what is clicked on it.
export function useOverlayControl() {
  const on = usePrefs((p) => p.overlay.on);
  const place = usePrefs((p) => p.overlay.place);
  const supported = isTauri() && !IS_LINUX;

  useEffect(() => {
    if (!supported) return;
    void (on ? invoke("overlay_show", { place }) : invoke("overlay_hide")).catch((err) => console.warn("floating bar:", err));
  }, [supported, on, place]);

  useEffect(() => {
    if (!supported) return;
    const off = listen<OverlayTarget | null>(OVERLAY_OPEN, (e) => {
      if (e.payload) void focusSession(e.payload.box, e.payload.session);
      else goHome();
    });
    return () => void off.then((f) => f());
  }, [supported]);
}

// What the bar shows comes from here, only while it is on: loaded then, so
// the app's startup doesn't carry it.
const Feed = lazy(() => import("@/overlay/feed"));

export function OverlayFeed() {
  const on = usePrefs((p) => p.overlay.on);
  if (!on || !isTauri() || IS_LINUX) return null;
  return (
    <Suspense fallback={null}>
      <Feed />
    </Suspense>
  );
}

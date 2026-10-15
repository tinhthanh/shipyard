import { invoke } from "@tauri-apps/api/core";
import { useEffect, useMemo } from "react";

import { usePrefs } from "@/lib/prefs";
import { barState } from "@/overlay/bar-state";
import { useLoginLimits } from "@/overlay/data";
import { useAgentRows } from "@/views/home/widgets/agents";

// OverlayFeed tells the floating bar (app/bar) what to show, each time that
// changes: each login's plan limits, read with the Usage plugin's script
// while that plugin is on, and the agents that need you or are working.
// The main window renders it while the bar is on (overlay/control.ts).
export default function OverlayFeed() {
  const usage = usePrefs((p) => p.enabledPlugins.includes("usage") && !p.disabledPlugins.includes("usage"));
  const logins = useLoginLimits(usage);
  const rows = useAgentRows();
  const state = useMemo(() => barState(usage, logins, rows), [usage, logins, rows]);
  const line = JSON.stringify(state);
  useEffect(() => {
    void invoke("overlay_state", { state: JSON.parse(line) }).catch((err) => console.warn("floating bar:", err));
  }, [line]);
  return null;
}

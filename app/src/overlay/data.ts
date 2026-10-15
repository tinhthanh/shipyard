import { useEffect } from "react";

import { NoProjects, type Report, runScript, where } from "../../../plugins/usage/src/box.ts";
import { type LoginLimits, limitsByLogin } from "../../../plugins/usage/src/summary.ts";
import { endpoint, httpClient } from "@/lib/api";
import { refreshFor } from "@/lib/events";
import { useStore } from "@/lib/store";
import { useWidgetDataIn } from "@/lib/widget-data";
import { isMock } from "@/hooks/use-berth-connection";
import { makeContext, pluginStorage } from "@/plugins/context";

// A box that couldn't be read (its agent restarting, a slow link) is tried
// again this soon, rather than at the next 5-minute read.
const RETRY_FAILED = 30_000;

// The floating bar's own connection to the agent: the store's boxes and
// sessions, kept fresh by the event stream, and nothing else. Notifications,
// plugins and runs stay with the main window, which already shows them.
export function useOverlayConnection() {
  useEffect(() => {
    const abort = new AbortController();
    let poll = 0;
    void (async () => {
      let delay = 1000;
      while (!abort.signal.aborted) {
        try {
          const client = isMock() ? (await import("@/lib/mock")).mockClient() : httpClient(await endpoint());
          if (!isMock()) await client.status();
          if (abort.signal.aborted) return;
          const { setClient, refreshAll } = useStore.getState();
          setClient(client);
          void refreshAll();
          client.events(refreshFor, () => void refreshAll(), abort.signal);
          poll = window.setInterval(() => void useStore.getState().refreshAll(), 60_000);
          return;
        } catch {
          await new Promise((r) => setTimeout(r, delay));
          delay = Math.min(delay * 2, 10_000);
        }
      }
    })();
    return () => {
      abort.abort();
      window.clearInterval(poll);
    };
  }, []);
}

// Whether the Usage & accounts plugin is on, from the prefs the main window
// saves (the bar only reads them).
export function usageOn(): boolean {
  try {
    const p = JSON.parse(localStorage.getItem("berth.prefs") ?? "{}") as { enabledPlugins?: string[]; disabledPlugins?: string[] };
    return !!p.enabledPlugins?.includes("usage") && !p.disabledPlugins?.includes("usage");
  } catch {
    return false;
  }
}

// useLoginLimits reads each online box's agent records with the Usage
// plugin's own script, every 5 minutes, and gives each login's open plan
// windows. A box that can't be read is left out, and tried again soon.
export function useLoginLimits(on: boolean): LoginLimits[] {
  const client = useStore((s) => s.client);
  const online = useStore((s) => (s.status?.boxes ?? []).filter((b) => b.state === "online").map((b) => b.name).join(","));
  const key = on && client && online ? `limits:${online}` : "limits:off";
  const { data, refresh } = useWidgetDataIn<{ logins: LoginLimits[]; failed: number }>(
    pluginStorage("usage"),
    "usage",
    key,
    async () => {
      if (key === "limits:off" || !client) return { logins: [], failed: 0 };
      const ctx = makeContext("usage", client);
      const got = await Promise.all(
        online.split(",").map(async (box) => {
          try {
            return { report: await runScript<Report>(ctx, box, await where(ctx, box), ["report", "7"], "2m") };
          } catch (err) {
            // A box with no projects has nothing to read: not a failure.
            return err instanceof NoProjects ? null : undefined;
          }
        }),
      );
      return { logins: limitsByLogin(got.filter((r): r is { report: Report } => !!r)), failed: got.filter((r) => r === undefined).length };
    },
    { every: 5 * 60_000 },
  );
  const failed = data?.failed ?? 0;
  useEffect(() => {
    if (!failed) return;
    const t = window.setTimeout(refresh, RETRY_FAILED);
    return () => window.clearTimeout(t);
    // refresh is a new function each render; the read it starts is what matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [failed, data]);
  return data?.logins ?? [];
}

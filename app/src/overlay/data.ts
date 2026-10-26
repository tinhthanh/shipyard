import { useEffect } from "react";

import { NoProjects, type Report, runScript, where } from "../../../plugins/usage/src/box.ts";
import { type LoginLimits, limitsByLogin, signedOutLogins } from "../../../plugins/usage/src/summary.ts";
import { useStore } from "@/lib/store";
import { useWidgetDataIn } from "@/lib/widget-data";
import { makeContext, pluginStorage } from "@/plugins/context";

// A box that couldn't be read (its agent restarting, a slow link) is tried
// again this soon, rather than at the next 5-minute read.
const RETRY_FAILED = 30_000;

// useLoginLimits reads each online box's agent records with the Usage
// plugin's own script, every 5 minutes, and gives each login's open plan
// windows, and the logins that need signing in again. A box that can't be
// read is left out, and tried again soon.
export function useLoginLimits(on: boolean): { logins: LoginLimits[]; signedOut: string[] } {
  const client = useStore((s) => s.client);
  const online = useStore((s) => (s.status?.boxes ?? []).filter((b) => b.state === "online").map((b) => b.name).join(","));
  const key = on && client && online ? `limits:${online}` : "limits:off";
  const { data, refresh } = useWidgetDataIn<{ logins: LoginLimits[]; signedOut?: string[]; failed: number }>(
    pluginStorage("usage"),
    "usage",
    key,
    async () => {
      if (key === "limits:off" || !client) return { logins: [], signedOut: [], failed: 0 };
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
      const read = got.filter((r): r is { report: Report } => !!r);
      return { logins: limitsByLogin(read), signedOut: signedOutLogins(read), failed: got.filter((r) => r === undefined).length };
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
  return { logins: data?.logins ?? [], signedOut: data?.signedOut ?? [] };
}

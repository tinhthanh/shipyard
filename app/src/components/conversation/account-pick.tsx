import { UserRoundIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { type Accounts, runScript, where } from "../../../../plugins/usage/src/box.ts";
import { Pick, type PickOption } from "@/components/conversation/composer-pickers";
import { agentPresets } from "@/lib/actions";
import { usePrefs } from "@/lib/prefs";
import type { AgentPick } from "@/lib/composer";
import { useStore } from "@/lib/store";
import { useWidgetDataIn } from "@/lib/widget-data";
import { makeContext, pluginStorage } from "@/plugins/context";

// The account a new task's agent uses, for that task alone (berthd's
// taskaccounts.go): New task's Account picker, beside Agents. "" is the
// project's (or the box's) own pick, as before; the list is the box's
// Claude Code or Codex accounts, read with the Usage plugin's script.
// Shown only with the Usage plugin on and one Claude Code or Codex agent.

type Kind = "claude" | "codex";

// kindOf is which accounts an agent preset uses, by the program it runs.
function kindOf(box: string, location: string, agent: string): Kind | undefined {
  const preset = agentPresets(box, location).find((p) => p.id === agent);
  const program = (preset?.command ?? agent).trim().split(/\s+/)[0]?.split("/").pop() ?? "";
  return program === "claude" || program === "codex" ? program : undefined;
}

export function useAccountPick(box: string, location: string, picks: AgentPick[], on: boolean) {
  const usage = usePrefs((p) => p.enabledPlugins.includes("usage") && !p.disabledPlugins.includes("usage"));
  const client = useStore((s) => s.client);
  const kind = on && picks.length === 1 && box && location ? kindOf(box, location, picks[0].agent) : undefined;
  const key = usage && client && kind ? `accounts:${box}` : "accounts:off";
  const { data } = useWidgetDataIn<Accounts | null>(
    pluginStorage("usage"),
    "usage",
    key,
    async () => {
      if (key === "accounts:off" || !client) return null;
      const ctx = makeContext("usage", client);
      return runScript<Accounts>(ctx, box, await where(ctx, box, location), ["accounts"], "30s").catch(() => null);
    },
    { every: 5 * 60_000 },
  );
  const [value, setValue] = useState("");
  // Another box or agent: back to the project's own.
  useEffect(() => setValue(""), [box, kind]);

  const options = useMemo<PickOption[]>(() => {
    const list = (data?.accounts ?? []).filter((a) => a.agent === kind && a.exists);
    return [
      { value: "", label: "Project's account", detail: "as set in Usage" },
      ...list.map((a) => ({
        value: a.dir,
        label: a.id,
        detail: a.signed_out ? "signed out" : a.signed_in ? (a.email ?? undefined) : "not signed in",
        disabled: !a.signed_in,
      })),
    ];
  }, [data, kind]);

  const show = !!kind && options.length > 2;
  return {
    element: show ? <Pick label="Account" icon={<UserRoundIcon />} value={value} options={options} onPick={setValue} /> : null,
    // The request's "accounts", when one is picked.
    accounts: show && kind && value ? { [kind]: value } : undefined,
  };
}

import { ArrowRightLeftIcon, UserRoundIcon } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { create } from "zustand";

import { type Account, type Accounts, runScript, where } from "../../../../plugins/usage/src/box.ts";
import type { LoginLimits } from "../../../../plugins/usage/src/summary.ts";
import { Tip } from "@/components/tip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogDescription, DialogFooter, DialogHeader, DialogPanel, DialogPopup, DialogTitle } from "@/components/ui/dialog";
import { toastManager } from "@/components/ui/toast";
import { boxApi } from "@/lib/api";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { useLoginLimits } from "@/overlay/data";
import { makeContext } from "@/plugins/context";

// Which account a Claude session signs in with, and carrying its
// conversation on with another (the fork's; berthd's sessionaccount.go):
// the account chip under the reply box, the "Use another account" step on a
// usage limit's notice, and the dialog both open. The dialog lists the
// box's Claude accounts in the order to use them (the floating bar's: the
// week that would go unused soonest first, a spent 5-hour held back, a full
// one last) and moves the conversation to the one picked.

export const useAccountSwitch = create<{ target?: { box: string; session: string } }>(() => ({}));
export const openAccountSwitch = (box: string, session: string) => useAccountSwitch.setState({ target: { box, session } });

const useSession = (box: string, session: string) => useStore((s) => s.boxes[box]?.sessions?.find((x) => x.name === session));
export const canSwitchAccount = (box: string) => !!useStore.getState().boxes[box]?.info?.capabilities?.includes("session.account.switch");

// AccountChip says which account the session signs in with; a click opens
// the dialog, where the box can move it.
export function AccountChip({ box, session, agent, className }: { box: string; session: string; agent: string; className: string }) {
  const account = useSession(box, session)?.account;
  const movable = useStore((s) => !!s.boxes[box]?.info?.capabilities?.includes("session.account.switch")) && agent === "claude";
  if (!account) return null;
  const chip = (
    <span className="flex min-w-0 items-center gap-1">
      <UserRoundIcon className="size-3.5 shrink-0" />
      <span data-testid="account-chip-name" className="truncate">
        {account}
      </span>
    </span>
  );
  return (
    <Tip label={movable ? `Signed in as ${account}. Continue on another account…` : `Signed in as ${account}`}>
      {movable ? (
        <button type="button" data-testid="account-chip" aria-label={`Account: ${account}`} className={className} onClick={() => openAccountSwitch(box, session)}>
          {chip}
        </button>
      ) : (
        <span data-testid="account-chip" aria-label={`Account: ${account}`} className={className}>
          {chip}
        </span>
      )}
    </Tip>
  );
}

// --- The order to use them, as the floating bar has it (Model.swift) ---

interface Rank {
  score: number;
  // When a full one is back, ms; undefined when it isn't full.
  back?: number;
  five?: number;
  week?: number;
}

export function rank(l: LoginLimits | undefined, now = Date.now()): Rank | undefined {
  if (!l) return undefined;
  const plain = l.windows.filter((w) => !w.name.includes("("));
  const pool = plain.length ? plain : l.windows;
  if (!pool.length) return undefined;
  const week = pool.reduce((a, b) => (a.minutes !== b.minutes ? (a.minutes > b.minutes ? a : b) : a.used >= b.used ? a : b));
  const shortest = pool.reduce((a, b) => (a.minutes <= b.minutes ? a : b));
  const session = shortest.minutes < week.minutes ? shortest : undefined;
  const hours = (w: { minutes: number; resets: number }) => (w.resets > 0 ? Math.max(0.25, (w.resets - now) / 3_600_000) : w.minutes / 60);
  const need = Math.max(0, 1 - week.used) / hours(week);
  const headroom = session ? Math.max(0, 1 - session.used) : 1;
  const full = pool.filter((w) => w.used >= 0.95);
  return { score: need * headroom, back: full.length ? Math.max(...full.map((w) => (w.resets > 0 ? w.resets : now))) : undefined, five: session?.used, week: week.used };
}

const loginOf = (a: Account) => (a.email && a.org_id ? `${a.email}#${a.org_id}` : a.email || a.id);
const pct = (v?: number) => (v === undefined ? "–" : `${Math.round(v * 100)}%`);

function inTime(ms: number) {
  const m = Math.max(0, Math.round((ms - Date.now()) / 60_000));
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  return h < 24 ? `${h}h ${m % 60}m` : `${Math.floor(h / 24)}d ${h % 24}h`;
}

// --- The dialog ---

export function AccountSwitchDialog() {
  const target = useAccountSwitch((s) => s.target);
  const close = () => useAccountSwitch.setState({ target: undefined });
  return (
    <Dialog open={!!target} onOpenChange={(o: boolean) => !o && close()}>
      {target && <SwitchBody key={`${target.box}/${target.session}`} box={target.box} session={target.session} close={close} />}
    </Dialog>
  );
}

function SwitchBody({ box, session, close }: { box: string; session: string; close: () => void }) {
  const client = useStore((s) => s.client);
  const sess = useSession(box, session);
  const current = sess?.account;
  const working = sess?.agent_state === "running";
  const [accounts, setAccounts] = useState<Account[]>();
  const [problem, setProblem] = useState<string>();
  const [picked, setPicked] = useState<string>();
  const [carryOn, setCarryOn] = useState(true);
  const [busy, setBusy] = useState(false);
  const { logins } = useLoginLimits(true);

  useEffect(() => {
    if (!client) return;
    let gone = false;
    (async () => {
      const ctx = makeContext("usage", client);
      const got = await runScript<Accounts>(ctx, box, await where(ctx, box, sess?.location), ["accounts"], "30s");
      if (!gone) setAccounts(got.accounts.filter((a) => a.agent === "claude" && a.exists));
    })().catch((err) => !gone && setProblem(String((err as Error).message ?? err)));
    return () => {
      gone = true;
    };
  }, [client, box, sess?.location]);

  const rows = useMemo(() => {
    const now = Date.now();
    const of = new Map(logins.filter((l) => l.agent === "claude").map((l) => [l.login, l]));
    return (accounts ?? [])
      .filter((a) => a.id !== current)
      .map((a) => ({ a, r: rank(of.get(loginOf(a)), now), usable: a.signed_in && !a.signed_out }))
      .sort((x, y) => Number(y.usable) - Number(x.usable) || Number(!!x.r?.back) - Number(!!y.r?.back) || (x.r?.back ?? 0) - (y.r?.back ?? 0) || (y.r?.score ?? -1) - (x.r?.score ?? -1));
  }, [accounts, logins, current]);

  // The first that can take it, until one is picked.
  const choice = picked ?? rows.find((x) => x.usable && !x.r?.back)?.a.id ?? rows.find((x) => x.usable)?.a.id;
  const chosen = rows.find((x) => x.a.id === choice)?.a;
  const from = accounts?.find((a) => a.id === current);
  const otherOrg = !!chosen && !!from && (chosen.org_id ?? "") !== (from.org_id ?? "");

  const go = async () => {
    if (!client || !chosen) return;
    setBusy(true);
    setProblem(undefined);
    try {
      await boxApi.switchAccount(client, box, session, chosen.id === "default" ? "default" : chosen.dir, working);
      if (carryOn) await boxApi.send(client, box, session, "continue", true, { when: "idle" }).catch(() => {});
      toastManager.add({ title: `${session} is on ${chosen.id} now`, description: carryOn ? "Told it to continue." : "Its conversation carries on there.", type: "success" });
      close();
    } catch (err) {
      setProblem(String((err as Error).message ?? err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <DialogPopup className="sm:max-w-lg" data-testid="account-switch">
      <DialogHeader>
        <DialogTitle>Continue on another account</DialogTitle>
        <DialogDescription>
          {session} signs in as <span className="font-medium text-foreground">{current ?? "?"}</span>. Its conversation is copied to the account you pick, and Claude Code starts again there with it.
        </DialogDescription>
      </DialogHeader>
      <DialogPanel className="space-y-3">
        {!accounts && !problem && <p className="text-desc text-muted-foreground">Reading {box}'s accounts…</p>}
        {accounts && rows.length === 0 && <p className="text-desc text-muted-foreground">{box} has no other Claude Code account. Add one in Usage & accounts.</p>}
        <ul role="radiogroup" aria-label="Account" className="space-y-1">
          {rows.map(({ a, r, usable }, i) => {
            const on = a.id === choice;
            return (
              <li key={a.id}>
                <button
                  type="button"
                  role="radio"
                  aria-checked={on}
                  disabled={!usable || busy}
                  data-testid="account-switch-option"
                  data-account={a.id}
                  onClick={() => setPicked(a.id)}
                  className={cn(
                    "flex w-full items-center gap-2.5 rounded-lg border px-3 py-2 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-55",
                    on ? "border-foreground/30 bg-accent" : "border-transparent hover:bg-accent/60",
                  )}
                >
                  <span className={cn("size-3.5 shrink-0 rounded-full border", on ? "border-[4px] border-foreground" : "border-muted-foreground/50")} />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-1.5">
                      <span className="truncate font-medium text-body">{a.id}</span>
                      {i === 0 && usable && !r?.back && (
                        <Badge variant="success" size="sm">
                          use next
                        </Badge>
                      )}
                      {r?.back !== undefined && (
                        <Badge variant="error" size="sm">
                          full · back in {inTime(r.back)}
                        </Badge>
                      )}
                      {!usable && (
                        <Badge variant="warning" size="sm">
                          {a.signed_out ? "signed out" : "not signed in"}
                        </Badge>
                      )}
                    </span>
                    <span className="block truncate text-meta text-muted-foreground">{[a.email, a.org].filter(Boolean).join(" · ")}</span>
                  </span>
                  <span className="shrink-0 text-right text-meta text-muted-foreground tabular-nums">
                    5h {pct(r?.five)}
                    <br />
                    week {pct(r?.week)}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
        {chosen && (
          <ul className="list-disc space-y-1 pl-5 text-desc text-muted-foreground">
            <li>Its first turn there reads the whole conversation again, without the old account's cache: that counts on {chosen.id}'s limits.</li>
            {working && <li>It is at work now: it is stopped first.</li>}
            <li>Subagents and background commands it has running stop.</li>
            {otherOrg && <li className="text-warning-foreground">{chosen.id} is in another organisation than {current}: the conversation and the code it read go with it.</li>}
          </ul>
        )}
        {chosen && (
          <label className="flex items-center gap-2 text-desc">
            <Checkbox checked={carryOn} disabled={busy} onCheckedChange={(v: boolean) => setCarryOn(!!v)} />
            Then tell it to continue
          </label>
        )}
        {problem && <p className="text-desc text-destructive-foreground">{problem}</p>}
      </DialogPanel>
      <DialogFooter>
        <Button variant="ghost" onClick={close} disabled={busy}>
          Cancel
        </Button>
        <Button data-testid="account-switch-go" disabled={!chosen} loading={busy} onClick={() => void go()}>
          <ArrowRightLeftIcon />
          {chosen ? `Move to ${chosen.id}` : "Move"}
        </Button>
      </DialogFooter>
    </DialogPopup>
  );
}

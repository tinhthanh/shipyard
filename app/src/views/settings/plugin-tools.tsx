// A plugin's tools for agents, in Settings → Plugins (fork): its one-time
// setup on this computer, its tools on boxes, its settings, what it gives
// agents, and installing a plugin from a release. plugins-section.tsx shows
// them in each plugin's row.

import { RefreshCwIcon } from "lucide-react";
import { Fragment, useCallback, useEffect, useState } from "react";

import { ErrorText } from "@/components/error-note";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { toastManager } from "@/components/ui/toast";
import { ApiError, laptopApi, type PluginBoxes, type PluginInfo, type PluginOnBox, type PluginSetting, type PluginSettingsState, type PluginSetupState } from "@/lib/api";
import { plainError } from "@/lib/errors";
import { boxPollMs, boxRowView, outdatedBoxes } from "@/lib/plugin-boxes";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { ConfirmDialog } from "@/views/settings/confirm";

// PluginSetupRow shows whether a plugin's one-time setup on this computer is
// done, and offers it until it is, checking again every few seconds while it
// isn't (the person may be in System Settings right now).
export function PluginSetupRow({ plugin: p }: { plugin: PluginInfo }) {
  const [state, setState] = useState<PluginSetupState>();
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<string>();
  useEffect(() => {
    let live = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const check = async () => {
      const client = useStore.getState().client;
      if (!client) return;
      try {
        const s = await laptopApi.pluginSetup(client, p.id);
        if (!live) return;
        setState(s);
        if (!(s.known && s.ready)) timer = setTimeout(() => void check(), 3000);
      } catch (err) {
        if (live) setError(plainError(err));
      }
    };
    void check();
    return () => {
      live = false;
      if (timer) clearTimeout(timer);
    };
  }, [p.id]);
  const start = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    setStarting(true);
    setError(undefined);
    try {
      await laptopApi.startPluginSetup(client, p.id);
    } catch (err) {
      setError(plainError(err));
    } finally {
      setStarting(false);
    }
  };
  if (!state && !error) return null;
  if (state?.known && state.ready) {
    return (
      <div className="mt-1.5">
        <Badge variant="success" size="sm">
          Set up on this computer
        </Badge>
      </div>
    );
  }
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-2">
      <span className="text-warning-foreground text-meta">{state?.problem || state?.label || p.setup?.label || "Needs one more step on this computer"}</span>
      <Button size="xs" variant="outline" loading={starting} onClick={() => void start()}>
        Set up…
      </Button>
      {(error || state?.error) && <ErrorText className="text-destructive-foreground text-meta" text={error ?? state?.error ?? ""} />}
    </div>
  );
}

// PluginBoxesRows puts a plugin's tools on boxes too: for each online box,
// whether its agents have them, a switch to put them there or take them off,
// and what is left to do there (an update, or the plugin's setup, which
// opens on the box's screen for the person at it or reaching it remotely).
// Nothing shows for a plugin that gives agents no tools.
export function PluginBoxesRows({ plugin: p }: { plugin: PluginInfo }) {
  const [state, setState] = useState<PluginBoxes>();
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState<string>();
  // The box whose settings are open under its row.
  const [settingsOf, setSettingsOf] = useState<string>();
  const load = useCallback(async () => {
    const client = useStore.getState().client;
    if (!client) return;
    try {
      setState(await laptopApi.pluginBoxes(client, p.id));
    } catch {
      // A plugin that isn't one of this computer's, or an older agent:
      // nothing to offer.
      setState(undefined);
    }
  }, [p.id]);
  // When a setup was last started on a box: the rows look more often then.
  const [setupAt, setSetupAt] = useState<number>();
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>;
    let live = true;
    const tick = async () => {
      await load();
      if (live) timer = setTimeout(() => void tick(), boxPollMs(setupAt, Date.now()));
    };
    void tick();
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [load, setupAt]);
  if (!state?.tools || !state.boxes.length) return null;
  const act = async (row: PluginOnBox, what: "install" | "remove" | "setup") => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy(`${row.box}:${what}`);
    setError(undefined);
    try {
      if (what === "install") await laptopApi.installPluginOnBox(client, p.id, row.box);
      else if (what === "remove") await laptopApi.removePluginFromBox(client, p.id, row.box);
      else {
        const started = await laptopApi.startPluginSetupOnBox(client, p.id, row.box);
        setSetupAt(Date.now());
        toastManager.add(
          started.locked
            ? { type: "warning", title: `${row.box}'s screen is locked`, description: "Setup opened there, but it waits for someone to unlock the screen, there or by remote desktop." }
            : { type: "info", title: `Setup opened on ${row.box}`, description: "Finish it on that box's screen, there or by remote desktop." },
        );
      }
      await load();
    } catch (err) {
      setError(`${row.box}: ${plainError(err)}`);
    } finally {
      setBusy(undefined);
    }
  };
  const outdated = outdatedBoxes(state.boxes);
  // updateAll sends the version allowed here to every box with another, one
  // after the other, and says which it couldn't reach.
  const updateAll = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy("all:install");
    setError(undefined);
    const failed: string[] = [];
    for (const name of outdated) {
      try {
        await laptopApi.installPluginOnBox(client, p.id, name);
      } catch (err) {
        failed.push(`${name}: ${plainError(err)}`);
      }
    }
    if (failed.length) setError(failed.join("; "));
    await load();
    setBusy(undefined);
  };
  return (
    <div className="mt-2 space-y-1.5 rounded-md border px-2.5 py-2" data-testid={`plugin-boxes-${p.id}`}>
      <div className="flex items-center gap-2">
        <p className="min-w-0 flex-1 text-muted-foreground text-meta">On boxes: their agents get these tools too.</p>
        {outdated.length > 1 && (
          <Button size="xs" variant="outline" loading={busy === "all:install"} disabled={!!busy || !state.enabled} onClick={() => void updateAll()} data-testid={`plugin-boxes-update-all-${p.id}`}>
            Update all ({outdated.length})
          </Button>
        )}
      </div>
      {state.boxes.map((row) => {
        const v = boxRowView(row);
        return (
          <Fragment key={row.box}>
            <div className="flex items-center gap-2 text-meta" data-testid={`plugin-box-${row.box}`}>
              <Switch
                checked={v.on}
                disabled={!v.canToggle || !!busy || (!state.enabled && !v.on)}
                aria-label={`${p.name} on ${row.box}`}
                onCheckedChange={(on) => void act(row, on ? "install" : "remove")}
              />
              <span className="font-medium">{row.box}</span>
              <span className={cn("min-w-0 flex-1 truncate", v.tone === "success" ? "text-success-foreground" : v.tone === "warning" ? "text-warning-foreground" : v.tone === "error" ? "text-destructive-foreground" : "text-muted-foreground")}>{v.note}</span>
              {v.action === "update" && (
                <Button size="xs" variant="outline" loading={busy === `${row.box}:install`} disabled={!!busy || !state.enabled} onClick={() => void act(row, "install")}>
                  Update
                </Button>
              )}
              {v.action === "setup" && (
                <Button size="xs" variant="outline" loading={busy === `${row.box}:setup`} disabled={!!busy} onClick={() => void act(row, "setup")}>
                  Set up on box…
                </Button>
              )}
              {v.on && p.settings && (
                <Button size="xs" variant="ghost" aria-expanded={settingsOf === row.box} onClick={() => setSettingsOf(settingsOf === row.box ? undefined : row.box)} data-testid={`plugin-box-settings-toggle-${row.box}`}>
                  Settings
                </Button>
              )}
            </div>
            {v.on && p.settings && settingsOf === row.box && <PluginSettingsRows plugin={p} box={row.box} />}
          </Fragment>
        );
      })}
      {error && <ErrorText className="text-destructive-foreground text-meta" text={error} />}
    </div>
  );
}

// oldBoxError says a box's Shipyard is too old to have a plugin's settings
// there (it doesn't know the route), instead of a bare "not found".
function oldBoxError(err: unknown, box: string | undefined): string | undefined {
  if (box && err instanceof ApiError && err.status === 404) return `Update Shipyard on ${box} to change its settings here.`;
  return undefined;
}

// PluginSettingsRows lists the switches a plugin offers for its tools, such
// as which Chrome profiles agents may use: this computer's, or with box set,
// that box's own. Turning on one that carries a warning asks first.
export function PluginSettingsRows({ plugin: p, box, onChanged }: { plugin: PluginInfo; box?: string; onChanged?(): void }) {
  const [state, setState] = useState<PluginSettingsState>();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState<string>();
  const [confirming, setConfirming] = useState<PluginSetting>();
  const load = useCallback(async () => {
    const client = useStore.getState().client;
    if (!client) return;
    try {
      setState(box ? await laptopApi.pluginSettingsOnBox(client, p.id, box) : await laptopApi.pluginSettings(client, p.id));
      setError(undefined);
    } catch (err) {
      setError(oldBoxError(err, box) ?? plainError(err));
    }
  }, [p.id, box]);
  useEffect(() => {
    void load();
  }, [load]);
  const set = async (s: PluginSetting, value: boolean) => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy(s.key);
    try {
      setState(box ? await laptopApi.setPluginSettingOnBox(client, p.id, box, s.key, value) : await laptopApi.setPluginSetting(client, p.id, s.key, value));
      setError(undefined);
      // Something just turned on may need its one-time step (the Chrome
      // plugin opens the steps in a profile that lacks its extension).
      if (value && p.setup && !box) await laptopApi.startPluginSetup(client, p.id);
      onChanged?.();
    } catch (err) {
      setError(oldBoxError(err, box) ?? plainError(err));
    } finally {
      setBusy(undefined);
    }
  };
  if (!state && !error) return null;
  return (
    <div className={cn("rounded-md border", box ? "mt-1 mb-1 ml-9" : "mt-2")} data-testid={box ? `plugin-box-settings-${box}` : undefined}>
      <div className="flex items-center gap-2 border-b px-3 py-1.5 text-muted-foreground text-meta">
        <span className="flex-1">
          {state?.label || p.settings?.label || "Settings"}
          {box && ` on ${box}`}
        </span>
        <Button size="xs" variant="ghost" aria-label="Check again" onClick={() => void load()}>
          <RefreshCwIcon />
        </Button>
      </div>
      {state?.settings.length === 0 && <div className="px-3 py-2 text-muted-foreground text-meta">Nothing to set yet.</div>}
      {state?.settings.map((s) => (
        <div key={s.key} className="flex items-center gap-3 px-3 py-1.5">
          <div className="min-w-0 flex-1">
            <div className="text-meta">{s.label}</div>
            {s.description && <div className="text-muted-foreground text-tag">{s.description}</div>}
          </div>
          <Switch
            checked={s.value}
            disabled={busy === s.key}
            aria-label={`${s.value ? "Turn off" : "Turn on"} ${s.label}`}
            onCheckedChange={(on) => {
              if (on && s.warning) setConfirming(s);
              else void set(s, on);
            }}
          />
        </div>
      ))}
      {error && <ErrorText className="px-3 py-1.5 text-destructive-foreground text-meta" text={error} />}
      <ConfirmDialog
        open={!!confirming}
        onOpenChange={(open) => !open && setConfirming(undefined)}
        title={`Turn on ${confirming?.label ?? ""}${box ? ` on ${box}` : ""}?`}
        description={confirming?.warning ?? ""}
        confirm="Turn on"
        destructive
        onConfirm={async () => {
          if (confirming) await set(confirming, true);
        }}
      />
    </div>
  );
}

// AgentTools lists the MCP servers and skills a plugin gives agents on boxes.
export function AgentTools({ plugin: p }: { plugin: PluginInfo }) {
  const parts = [...(p.mcp ?? []).map((n) => `MCP: ${n}`), ...(p.skills ?? []).map((n) => `Skill: ${n}`)];
  if (!parts.length) return null;
  return (
    <div className="mt-1.5 flex flex-wrap gap-1">
      {parts.map((t) => (
        <Badge key={t} variant="outline" size="sm">
          {t}
        </Badge>
      ))}
    </div>
  );
}

// InstallFromURL installs a plugin from a release: the .tar.gz of its
// folder at an https address, checked against its SHA-256 (given here, or
// published beside it as <url>.sha256). Like the listing, it never turns the
// plugin on: it lands under Installed, off until you allow it.
export function InstallFromURL({ onDone, onClose }: { onDone: () => void; onClose: () => void }) {
  const [url, setURL] = useState("");
  const [sum, setSum] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [exists, setExists] = useState(false);
  const install = async (replace: boolean) => {
    const client = useStore.getState().client;
    if (!client || !url.trim()) return;
    setBusy(true);
    setError(undefined);
    try {
      const out = await laptopApi.installPluginFromURL(client, url.trim(), sum.trim(), replace);
      toastManager.add({
        type: "success",
        title: `${out.name || out.id}${out.version ? ` ${out.version}` : ""} ${out.replaced ? "replaced" : "installed"}`,
        description: "It's off until you allow it, below.",
      });
      setExists(false);
      onDone();
      onClose();
    } catch (err) {
      if (err instanceof ApiError && err.code === "plugin_exists") setExists(true);
      setError(plainError(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="mb-3 space-y-2 rounded-md border px-3 py-2.5" data-testid="plugin-install-url-form">
      <p className="text-muted-foreground text-meta">A plugin's release: the .tar.gz of its folder at an https:// address. Its SHA-256 is read from &lt;url&gt;.sha256 when you leave it empty.</p>
      <Input placeholder="https://github.com/…/releases/download/…/chrome-…-darwin-universal.tar.gz" value={url} onChange={(e) => setURL(e.target.value)} aria-label="The plugin's address" />
      <Input placeholder="SHA-256 (optional)" value={sum} onChange={(e) => setSum(e.target.value)} aria-label="The archive's SHA-256" className="font-mono" />
      <div className="flex flex-wrap items-center gap-2">
        <Button size="xs" loading={busy && !exists} disabled={busy || !url.trim()} onClick={() => void install(false)}>
          Install
        </Button>
        {exists && (
          <Button size="xs" variant="destructive-outline" loading={busy} disabled={busy} onClick={() => void install(true)}>
            Replace the installed one
          </Button>
        )}
        <Button size="xs" variant="ghost" disabled={busy} onClick={onClose}>
          Cancel
        </Button>
      </div>
      {error && <ErrorText className="text-destructive-foreground text-meta" text={error} />}
    </div>
  );
}

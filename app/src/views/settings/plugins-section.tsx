import { RefreshCwIcon } from "lucide-react";
import { Fragment, useCallback, useEffect, useState } from "react";

import { ErrorText } from "@/components/error-note";
import { Tip } from "@/components/tip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { toastManager } from "@/components/ui/toast";
import { ApiError, laptopApi, type PluginBoxes, type PluginInfo, type PluginOnBox, type PluginSetting, type PluginSettingsState, type PluginSetupState } from "@/lib/api";
import { plainError } from "@/lib/errors";
import { bytes, errorMessage } from "@/lib/format";
import { marketAction, marketNote, type Marketplace, type MarketPlugin } from "@/lib/marketplace";
import { openDocs, openUrl } from "@/lib/open-url";
import { boxPollMs, boxRowView, outdatedBoxes } from "@/lib/plugin-boxes";
import { heldNote, type PluginHelpers, stopNote, stoppedNote } from "@/lib/plugin-helpers";
import { builtinOn, setBuiltinOn, usePrefs } from "@/lib/prefs";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { askToAllow, type PluginHelper, readPluginHelper, usePluginConsent } from "@/plugins/consent";
import { builtinPlugins, loadPlugins } from "@/plugins/host";
import { useRegistry } from "@/plugins/registry";
import { ConfirmDialog } from "@/views/settings/confirm";
import { Code, SettingsGroup, SettingsPage } from "@/views/settings/rows";
import { APP_NAME } from "@/lib/platform";

export function PluginsSection() {
  const client = useStore((s) => s.client);
  const loaded = useRegistry((s) => s.plugins);
  const [installed, setInstalled] = useState<PluginInfo[]>();
  const [builtins, setBuiltins] = useState<PluginInfo[]>();
  const [reloading, setReloading] = useState(false);
  const [fromURL, setFromURL] = useState(false);
  useEffect(() => void builtinPlugins().then(setBuiltins), []);

  const refresh = useCallback(async () => {
    if (client) setInstalled(await client.plugins().catch(() => []));
  }, [client]);
  // Read again whenever the consent dialog closes: it also opens by itself,
  // for a plugin an update changed, and allowing it there turns it on.
  const consenting = usePluginConsent((s) => !!s.request);
  useEffect(() => {
    if (!consenting) void refresh();
  }, [refresh, consenting]);

  // The listing: what the agent read from the registry, merged with what is
  // installed here. error is in plain words; old is an agent from before
  // the marketplace.
  const [market, setMarket] = useState<{ list?: Marketplace; error?: string; old?: boolean; reading?: boolean }>({ reading: true });
  const browse = useCallback(
    async (fresh = false) => {
      if (!client) return;
      setMarket((m) => ({ ...m, reading: true }));
      try {
        setMarket({ list: await laptopApi.marketplace(client, fresh) });
      } catch (err) {
        if (err instanceof ApiError && err.status === 404 && !err.code) setMarket({ old: true });
        else setMarket({ error: plainError(err) });
      }
    },
    [client],
  );
  useEffect(() => void browse(), [browse]);

  const reload = async () => {
    if (!client) return;
    setReloading(true);
    await loadPlugins(client);
    await Promise.all([refresh(), browse()]);
    setReloading(false);
  };

  return (
    <SettingsPage
      title="Plugins"
      description={
        <>
          Plugins aren't sandboxed, so each stays off until you allow it.
        </>
      }
    >
      <SettingsGroup
        title="Installed"
        actions={
          <span className="flex items-center gap-3">
            <span className="font-mono text-muted-foreground text-mono">~/.berth/plugins</span>
            <Button size="xs" variant="ghost" onClick={() => setFromURL((v) => !v)} data-testid="plugin-install-url">
              Install from URL…
            </Button>
            <Button size="xs" variant="ghost" className="gap-1.5" loading={reloading} onClick={() => void reload()}>
              <RefreshCwIcon className="mr-0.5" /> Reload
            </Button>
          </span>
        }
      >
        {fromURL && <InstallFromURL onDone={() => void reload()} onClose={() => setFromURL(false)} />}
        {installed === undefined ? (
          <div className="py-6 text-center text-muted-foreground text-body">Loading…</div>
        ) : installed.length === 0 ? (
          <div className="py-6 text-center text-muted-foreground text-body">
            No plugins yet. Install one from Browse, or{" "}
            <button type="button" onClick={() => void openDocs("/guides/plugins")} className="underline underline-offset-2 hover:text-foreground">
              write one
            </button>
            .
          </div>
        ) : (
          installed.map((p) => <PluginRow key={p.id} plugin={p} status={loaded.find((l) => l.id === p.id)} onChanged={() => void reload()} />)
        )}
      </SettingsGroup>
      <SettingsGroup title="Built in" actions={<span className="text-desc text-muted-foreground">Same plugin API as yours; one of yours with the same id replaces it</span>}>
        {builtins === undefined ? (
          <div className="py-6 text-center text-muted-foreground text-body">Loading…</div>
        ) : builtins.length === 0 ? (
          <div className="py-6 text-center text-muted-foreground text-body">This build has no built-in plugins.</div>
        ) : (
          builtins.map((p) => (
            <BuiltinRow key={p.id} plugin={p} status={loaded.find((l) => l.id === p.id && l.builtin)} replaced={installed?.some((u) => u.id === p.id) ?? false} onChanged={() => void reload()} />
          ))
        )}
      </SettingsGroup>
      <SettingsGroup
        title="Browse"
        description={
          market.list ? (
            <>
              Plugins listed at <Code>{market.list.host}</Code>. Installing one puts it under Installed, off until you allow it.
            </>
          ) : (
            "Installing one puts it under Installed, off until you allow it."
          )
        }
        actions={
          !market.old && (
            <Button size="xs" variant="ghost" className="gap-1.5 text-muted-foreground" disabled={market.reading} onClick={() => void browse(true)}>
              <RefreshCwIcon className={cn(market.reading && "animate-spin")} /> Read the listing again
            </Button>
          )
        }
      >
        {market.old ? (
          <p className="py-3.5 text-muted-foreground text-body">The {APP_NAME} agent on this computer is older than this app, so it can't read the listing yet.</p>
        ) : market.error ? (
          <div className="flex items-center gap-4 py-3.5" data-testid="market-error">
            <ErrorText className="min-w-0 flex-1 text-destructive-foreground text-body" text={market.error} />
            <Button size="xs" variant="outline" className="min-w-24" loading={market.reading} onClick={() => void browse(true)}>
              Try again
            </Button>
          </div>
        ) : !market.list ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-body">Loading…</div>
        ) : market.list.plugins.length === 0 ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-body">Nothing is listed yet.</div>
        ) : (
          market.list.plugins.map((p) => <MarketRow key={p.id} plugin={p} was={installed?.find((u) => u.id === p.id)} onChanged={() => void reload()} onStale={() => void browse(true)} />)
        )}
      </SettingsGroup>
    </SettingsPage>
  );
}

// MarketRow is one listed plugin: what it is, and the one thing to do about
// it. Installing only puts it in ~/.berth/plugins; turning it on is the
// Installed row's switch, which asks first.
function MarketRow({ plugin: p, was, onChanged, onStale }: { plugin: MarketPlugin; was?: PluginInfo; onChanged(): void; onStale(): void }) {
  const [busy, setBusy] = useState(false);
  const [replacing, setReplacing] = useState(false);
  const action = marketAction(p);
  const note = marketNote(p);

  const install = async (replace = false) => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy(true);
    try {
      await laptopApi.installPlugin(client, p.id, p.version, replace);
      toastManager.add({
        title: action === "update" ? `Updated ${p.name} to ${p.version}` : `Installed ${p.name}`,
        description: was?.allowed ? "Shipyard asks again before the new code runs." : "It's off until you turn it on under Installed.",
        type: "success",
      });
      onChanged();
    } catch (err) {
      toastManager.add({ title: `Couldn't ${action === "update" ? "update" : "install"} ${p.name}`, description: errorMessage(err), type: "error" });
      // The row showed something that is no longer so: read the listing again.
      if (err instanceof ApiError && (err.code === "listing_changed" || err.code === "plugin_by_hand")) onStale();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex items-center gap-4 py-3.5" data-testid="market-plugin" data-plugin={p.id}>
      <span className="size-2 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 text-row">
          <span>{p.name}</span>
          <span className="text-muted-foreground text-meta">{p.version}</span>
        </div>
        {p.description && <div className="mt-0.5 text-desc text-muted-foreground">{p.description}</div>}
        <div className="mt-0.5 font-mono text-tag text-muted-foreground">
          {p.id}
          {p.author && ` · by ${p.author}`}
          {` · ${bytes(p.size)}`}
          {/* The agent keeps only https links; the app opens nothing else either. */}
          {p.homepage?.startsWith("https://") && (
            <>
              {" · "}
              <button type="button" onClick={() => void openUrl(p.homepage!)} className="underline underline-offset-2 hover:text-foreground">
                homepage
              </button>
            </>
          )}
        </div>
        {note && <div className="mt-1 text-muted-foreground text-meta">{note}</div>}
        {p.error && (
          <div className="mt-1 text-meta">
            <span className="text-destructive-foreground">It can't be installed:</span> <span className="text-muted-foreground">{p.error}.</span>
          </div>
        )}
      </div>
      {action === "installed" ? (
        <Badge variant="outline" size="sm">
          Installed
        </Badge>
      ) : action === "replace" ? (
        <Button size="xs" variant="outline" className="min-w-24" loading={busy} onClick={() => setReplacing(true)}>
          Replace…
        </Button>
      ) : action !== "unavailable" ? (
        <Button size="xs" variant="outline" className="min-w-24" loading={busy} onClick={() => void install()}>
          {action === "update" ? "Update" : "Install"}
        </Button>
      ) : null}
      <ConfirmDialog
        open={replacing}
        onOpenChange={setReplacing}
        destructive
        title={`Replace your ${p.name}?`}
        description={`~/.berth/plugins/${p.id} holds a plugin the listing didn't install, or one changed since. Its folder is deleted and ${p.name} ${p.version} from the listing takes its place, off until you allow it.`}
        confirm="Replace"
        onConfirm={() => install(true)}
      />
    </div>
  );
}

function PluginRow({ plugin: p, status, onChanged }: { plugin: PluginInfo; status?: { state: string; error?: string }; onChanged(): void }) {
  const [removing, setRemoving] = useState(false);
  // Never on until you allow it, and off again when it changes.
  const enabled = p.enabled === true;
  // Bumped when a setting changes, so the setup row checks again.
  const [settingsChanges, setSettingsChanges] = useState(0);
  const review = !enabled && (p.changed || status?.state === "review");
  // The agent counts a plugin's hooks; older builds listed them.
  const hooks = Array.isArray(p.hooks) ? p.hooks.length : typeof p.hooks === "number" ? (p.hooks as number) : 0;
  const error = p.error ?? status?.error;
  const state = review ? "review" : !enabled ? "off" : error ? "failed" : (status?.state ?? "loading");
  // The program it carries for boxes, if any: its manifest says, and the
  // row says so too, on or off.
  const [helper, setHelper] = useState<PluginHelper>();
  useEffect(() => {
    const client = useStore.getState().client;
    if (!client) return;
    let live = true;
    void readPluginHelper(client, p).then((h) => live && setHelper(h));
    return () => {
      live = false;
    };
    // Read again when its files change: a new version, or a new permission.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p.id, p.version, p.allowed, p.changed]);

  // A plugin's program doesn't outlive it: turning the plugin off, or
  // removing it, stops the program on the boxes. So both ask first, and say
  // where; and a plugin that is off for review says if its program was left
  // running meanwhile. where is the agent's word on which boxes have it.
  const [turningOff, setTurningOff] = useState(false);
  const [where, setWhere] = useState<PluginHelpers>();
  const asking = turningOff || removing;
  useEffect(() => {
    const client = useStore.getState().client;
    if (!client || !helper || (enabled && !asking)) return;
    let live = true;
    // An agent older than this answers nothing: the words then name no box.
    laptopApi.pluginHelpers(client, p.id).then(
      (h) => live && setWhere(h),
      () => live && setWhere(undefined),
    );
    return () => {
      live = false;
    };
  }, [p.id, helper, enabled, asking, p.changed, p.allowed]);
  const held = !enabled && helper ? heldNote(helper.name, where) : undefined;

  const turnOff = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    try {
      const out = await laptopApi.disablePlugin(client, p.id);
      const note = helper && stoppedNote(helper.name, out.helpers);
      if (note) toastManager.add({ title: enabled ? `Turned off ${p.name}` : `Stopped ${helper.name}`, description: note, type: "success" });
      setWhere(undefined);
      onChanged();
    } catch (err) {
      toastManager.add({ title: `Couldn't turn off ${p.name}`, description: errorMessage(err), type: "error" });
    }
  };

  return (
    // A plugin with agent tools grows rows under it (setup, settings, boxes):
    // its controls stay by its name rather than float down the middle.
    <div className={cn("flex gap-4 py-4", enabled && (p.mcp?.length || p.skills?.length) ? "items-start" : "items-center")} data-testid="plugin-row" data-plugin={p.id}>
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 text-row">
          <StatusDot state={state} />
          <span>{p.name}</span>
          <span className="text-muted-foreground text-meta">{p.version}</span>
        </div>
        {p.description && <div className="mt-0.5 text-desc text-muted-foreground">{p.description}</div>}
        <Contributions plugin={p.id} />
        <AgentTools plugin={p} />
        {enabled && p.setup && <PluginSetupRow key={settingsChanges} plugin={p} />}
        {enabled && p.settings && <PluginSettingsRows plugin={p} onChanged={() => setSettingsChanges((n) => n + 1)} />}
        {enabled && <PluginBoxesRows plugin={p} />}
        <div className="mt-0.5 font-mono text-tag text-muted-foreground">
          {p.id}
          {hooks > 0 && ` · ${hooks} hook${hooks === 1 ? "" : "s"}`}
          {p.source === "marketplace" && " · from the listing"}
        </div>
        {helper && (
          <div className="mt-1 text-muted-foreground text-meta" data-testid="plugin-helper">
            It can also install and run a program, <Code>{helper.name}</Code>, on the boxes you turn it on for.
          </div>
        )}
        {review && (
          <div className="mt-1 text-meta">
            <span className="text-warning-foreground">Changed since you allowed it, so it's off.</span>{" "}
            <button type="button" onClick={() => void askToAllow(p).then((ok) => ok && onChanged())} className="underline underline-offset-2 hover:text-foreground">
              Review it
            </button>
          </div>
        )}
        {held && (
          <div className="mt-1 text-muted-foreground text-meta" data-testid="plugin-helper-held">
            {held}{" "}
            <button type="button" onClick={() => setTurningOff(true)} className="underline underline-offset-2 hover:text-foreground">
              Stop it
            </button>
          </div>
        )}
        {error && enabled && status?.state !== "review" && (
          <div className="mt-1 text-meta">
            <span className="text-destructive-foreground">{error}</span> <span className="text-muted-foreground">Fix it in ~/.berth/plugins/{p.id}, then Reload.</span>
          </div>
        )}
      </div>
      {/* One the listing installed is the app's to remove; a folder of your own is yours. */}
      {p.source === "marketplace" && (
        <Button size="xs" variant="ghost" className="text-muted-foreground" onClick={() => setRemoving(true)}>
          Remove…
        </Button>
      )}
      <ConfirmDialog
        open={removing}
        onOpenChange={setRemoving}
        destructive
        title={`Remove ${p.name}?`}
        description={`Its folder, ~/.berth/plugins/${p.id}, is deleted and what you allowed is forgotten. ${helper ? `${stopNote(helper.name, where, true)} ` : ""}You can install it again from Browse.`}
        confirm="Remove"
        onConfirm={async () => {
          const client = useStore.getState().client;
          if (!client) return;
          try {
            const out = await laptopApi.uninstallPlugin(client, p.id);
            toastManager.add({ title: `Removed ${p.name}`, description: helper && stoppedNote(helper.name, out.helpers), type: "success" });
            onChanged();
          } catch (err) {
            toastManager.add({ title: `Couldn't remove ${p.name}`, description: errorMessage(err), type: "error" });
          }
        }}
      />
      {helper && (
        <ConfirmDialog
          open={turningOff}
          onOpenChange={setTurningOff}
          title={enabled ? `Turn off ${p.name}?` : `Stop ${helper.name}?`}
          description={`${stopNote(helper.name, where)} Turning ${p.name} on again doesn't start it: you turn it on for each box again.`}
          confirm={enabled ? "Turn off" : "Stop it"}
          onConfirm={turnOff}
        />
      )}
      <Switch
        checked={enabled}
        aria-label={enabled ? `Turn off ${p.name}` : `Turn on ${p.name}`}
        onCheckedChange={async (on) => {
          const client = useStore.getState().client;
          if (!client) return;
          // Turning a plugin on asks first: see plugin-consent-dialog. So
          // does turning off one whose program runs on boxes.
          if (!on && helper) return setTurningOff(true);
          if (!on) return turnOff();
          try {
            if (await askToAllow(p)) onChanged();
          } catch (err) {
            toastManager.add({ title: `Couldn't turn on ${p.name}`, description: errorMessage(err), type: "error" });
          }
        }}
      />
    </div>
  );
}

function BuiltinRow({ plugin: p, status, replaced, onChanged }: { plugin: PluginInfo; status?: { state: string; error?: string }; replaced: boolean; onChanged(): void }) {
  const off = usePrefs((s) => !builtinOn(p, s));
  const state = replaced || off ? "off" : status?.error ? "failed" : (status?.state ?? "loading");
  return (
    <div className="flex items-center gap-4 py-4">
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 text-row">
          <StatusDot state={state} />
          <span>{p.name}</span>
          <span className="font-normal text-muted-foreground text-meta">{p.version}</span>
        </div>
        {p.description && <div className="mt-0.5 text-desc text-muted-foreground">{p.description}</div>}
        <Contributions plugin={p.id} />
        {p.defaultEnabled === false && off && !replaced && <div className="mt-0.5 text-muted-foreground text-meta">Off until you turn it on.</div>}
        {replaced && <div className="mt-0.5 text-muted-foreground text-meta">Replaced by your plugin with the same id.</div>}
        {status?.error && !off && <div className="mt-1 text-destructive-foreground text-meta">{status.error}</div>}
      </div>
      <Switch
        checked={!off}
        disabled={replaced}
        aria-label={off ? `Turn on ${p.name}` : `Turn off ${p.name}`}
        onCheckedChange={(on) => {
          setBuiltinOn(p, on);
          onChanged();
        }}
      />
    </div>
  );
}

// StatusDot marks only what needs attention, before the name: the switch
// already says on.
function StatusDot({ state }: { state: string }) {
  if (state === "active" || state === "off") return null;
  return (
    <Tip label={state}>
      <span role="img" aria-label={state} className={cn("size-1.5 shrink-0 self-center rounded-full", state === "failed" ? "bg-destructive" : "bg-warning")} />
    </Tip>
  );
}

// PluginSetupRow shows whether a plugin's one-time setup on this computer is
// done, and offers it until it is, checking again every few seconds while it
// isn't (the person may be in System Settings right now).
function PluginSetupRow({ plugin: p }: { plugin: PluginInfo }) {
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
function PluginBoxesRows({ plugin: p }: { plugin: PluginInfo }) {
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
function PluginSettingsRows({ plugin: p, box, onChanged }: { plugin: PluginInfo; box?: string; onChanged?(): void }) {
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
function AgentTools({ plugin: p }: { plugin: PluginInfo }) {
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

// Contributions lists what a loaded plugin adds to the app.
function Contributions({ plugin }: { plugin: string }) {
  const r = useRegistry();
  const mine = <T,>(list: { plugin: string; item: T }[]) => list.filter((c) => c.plugin === plugin).map((c) => c.item);
  const parts = [
    ...mine(r.sidebarItems).map((i) => `Sidebar: ${i.title}`),
    ...mine(r.worktreePanels).map((i) => `Panel: ${i.title}`),
    ...mine(r.worktreeSections).map((i) => `Worktree section${i.title ? `: ${i.title}` : ""}`),
    ...mine(r.homeWidgets).map((i) => `Home widget: ${i.title}`),
    ...(mine(r.statusBarItems).length ? ["Status bar"] : []),
    ...(mine(r.commands).length ? [`${mine(r.commands).length} command${mine(r.commands).length === 1 ? "" : "s"}`] : []),
    ...(mine(r.themes).length ? [`${mine(r.themes).length} theme${mine(r.themes).length === 1 ? "" : "s"}`] : []),
  ];
  if (!parts.length) return null;
  return <div className="mt-0.5 text-muted-foreground text-meta">{parts.join(" · ")}</div>;
}

// InstallFromURL installs a plugin from a release: the .tar.gz of its
// folder at an https address, checked against its SHA-256 (given here, or
// published beside it as <url>.sha256). Like the listing, it never turns the
// plugin on: it lands under Installed, off until you allow it.
function InstallFromURL({ onDone, onClose }: { onDone: () => void; onClose: () => void }) {
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

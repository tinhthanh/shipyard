import { RefreshCwIcon } from "lucide-react";
import { useCallback, useEffect, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { toastManager } from "@/components/ui/toast";
import { laptopApi, type PluginInfo } from "@/lib/api";
import { errorMessage } from "@/lib/format";
import { openDocs } from "@/lib/open-url";
import { builtinOn, setBuiltinOn, usePrefs } from "@/lib/prefs";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { askToAllow } from "@/plugins/consent";
import { builtinPlugins, loadPlugins } from "@/plugins/host";
import { useRegistry } from "@/plugins/registry";
import { Code, SettingsGroup, SettingsPage } from "@/views/settings/rows";
import { Tip } from "@/components/tip";

export function PluginsSection() {
  const client = useStore((s) => s.client);
  const loaded = useRegistry((s) => s.plugins);
  const [installed, setInstalled] = useState<PluginInfo[]>();
  const [builtins, setBuiltins] = useState<PluginInfo[]>();
  const [reloading, setReloading] = useState(false);
  useEffect(() => void builtinPlugins().then(setBuiltins), []);

  const refresh = useCallback(async () => {
    if (client) setInstalled(await client.plugins().catch(() => []));
  }, [client]);
  useEffect(() => void refresh(), [refresh]);

  const reload = async () => {
    if (!client) return;
    setReloading(true);
    await loadPlugins(client);
    await refresh();
    setReloading(false);
  };

  return (
    <SettingsPage
      title="Plugins"
      description={
        <>
          Plugins live in <Code>~/.berth/plugins/&lt;id&gt;/</Code> beside a <Code>berth-plugin.json</Code>. They aren't sandboxed: a plugin runs with the app's access to every box, and its hooks run on this computer. Each stays off until you allow it, and Shipyard asks again whenever its code changes.
        </>
      }
    >
      <SettingsGroup title="Built in" description="Ship with Shipyard and use the same plugin API as yours. A plugin of yours with the same id replaces one.">
        {builtins === undefined ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-sm">Loading…</div>
        ) : builtins.length === 0 ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-sm">This build has no built-in plugins.</div>
        ) : (
          builtins.map((p) => (
            <BuiltinRow key={p.id} plugin={p} status={loaded.find((l) => l.id === p.id && l.builtin)} replaced={installed?.some((u) => u.id === p.id) ?? false} onChanged={() => void reload()} />
          ))
        )}
      </SettingsGroup>
      <SettingsGroup
        title="Installed"
        actions={
          <Button size="xs" variant="outline" className="gap-1.5" loading={reloading} onClick={() => void reload()}>
            <RefreshCwIcon className="mr-0.5" /> Reload
          </Button>
        }
      >
        {installed === undefined ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-sm">Loading…</div>
        ) : installed.length === 0 ? (
          <div className="px-4 py-6 text-center text-muted-foreground text-sm">
            No plugins yet.{" "}
            <button type="button" onClick={() => void openDocs("/guides/plugins")} className="underline underline-offset-2 hover:text-foreground">
              Write one
            </button>
            .
          </div>
        ) : (
          installed.map((p) => <PluginRow key={p.id} plugin={p} status={loaded.find((l) => l.id === p.id)} onChanged={() => void reload()} />)
        )}
      </SettingsGroup>
    </SettingsPage>
  );
}

function PluginRow({ plugin: p, status, onChanged }: { plugin: PluginInfo; status?: { state: string; error?: string }; onChanged(): void }) {
  // Never on until you allow it, and off again when it changes.
  const enabled = p.enabled === true;
  const review = !enabled && (p.changed || status?.state === "review");
  // The agent counts a plugin's hooks; older builds listed them.
  const hooks = Array.isArray(p.hooks) ? p.hooks.length : typeof p.hooks === "number" ? (p.hooks as number) : 0;
  const error = p.error ?? status?.error;
  const state = review ? "review" : !enabled ? "off" : error ? "failed" : (status?.state ?? "loading");

  return (
    <div className="flex items-center gap-4 px-4 py-3">
      <StatusDot state={state} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2 text-sm">
          <span>{p.name}</span>
          <span className="text-muted-foreground text-xs">{p.version}</span>
        </div>
        {p.description && <div className="mt-0.5 text-muted-foreground text-xs">{p.description}</div>}
        <Contributions plugin={p.id} />
        <AgentTools plugin={p} />
        <div className="mt-0.5 font-mono text-[11px] text-muted-foreground/80">
          {p.id}
          {hooks > 0 && ` · ${hooks} hook${hooks === 1 ? "" : "s"}`}
        </div>
        {review && (
          <div className="mt-1 text-xs">
            <span className="text-warning-foreground">Changed since you allowed it, so it's off.</span>{" "}
            <button type="button" onClick={() => void askToAllow(p).then((ok) => ok && onChanged())} className="underline underline-offset-2 hover:text-foreground">
              Review it
            </button>
          </div>
        )}
        {error && enabled && status?.state !== "review" && (
          <div className="mt-1 text-xs">
            <span className="text-destructive-foreground">{error}</span> <span className="text-muted-foreground">Fix it in ~/.berth/plugins/{p.id}, then Reload.</span>
          </div>
        )}
      </div>
      <Switch
        checked={enabled}
        aria-label={enabled ? `Turn off ${p.name}` : `Turn on ${p.name}`}
        onCheckedChange={async (on) => {
          const client = useStore.getState().client;
          if (!client) return;
          try {
            // Turning a plugin on asks first: see plugin-consent-dialog.
            if (on) {
              if (!(await askToAllow(p))) return;
            } else await laptopApi.disablePlugin(client, p.id);
            onChanged();
          } catch (err) {
            toastManager.add({ title: `Couldn't turn ${on ? "on" : "off"} ${p.name}`, description: errorMessage(err), type: "error" });
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
    <div className="flex items-center gap-4 px-4 py-3">
      <StatusDot state={state} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 text-sm">
          <span>{p.name}</span>
          <Badge variant="outline" size="sm">Built-in</Badge>
          <span className="text-muted-foreground text-xs">{p.version}</span>
        </div>
        {p.description && <div className="mt-0.5 text-muted-foreground text-xs">{p.description}</div>}
        <Contributions plugin={p.id} />
        {p.defaultEnabled === false && off && !replaced && <div className="mt-0.5 text-muted-foreground text-xs">Off until you turn it on.</div>}
        {replaced && <div className="mt-0.5 text-muted-foreground text-xs">Replaced by your plugin with the same id.</div>}
        {status?.error && !off && <div className="mt-1 text-destructive-foreground text-xs">{status.error}</div>}
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

// StatusDot marks only what needs attention: the switch already says on.
function StatusDot({ state }: { state: string }) {
  if (state === "active" || state === "off") return <span className="size-2 shrink-0" />;
  return (
    <Tip label={state}>
      <span role="img" aria-label={state} className={cn("size-2 shrink-0 rounded-full", state === "failed" ? "bg-destructive" : "bg-warning")} />
    </Tip>
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
  return (
    <div className="mt-1.5 flex flex-wrap gap-1">
      {parts.map((p) => (
        <Badge key={p} variant="secondary" size="sm">
          {p}
        </Badge>
      ))}
    </div>
  );
}

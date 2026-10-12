import * as React from "react";
import * as ReactDOM from "react-dom";
import * as ReactJSXRuntime from "react/jsx-runtime";

import type { Client, PluginInfo } from "@/lib/api";
import { plainError } from "@/lib/errors";
import { builtinOn } from "@/lib/prefs";
import { askToAllow, mayLoad, readPluginFiles, usePluginConsent } from "@/plugins/consent";
import { makeContext } from "@/plugins/context";
import { pluginContexts } from "@/plugins/plugin-boundary";
import { removePlugin, setPluginOrder, setPluginStatus, usePluginsLoading, useRegistry } from "@/plugins/registry";
import * as sdk from "@/plugins/sdk-runtime";
import { pluginUi } from "@/plugins/ui";

// Plugins are ES modules that import react, react-dom and @berth/plugin by
// name. They must get the app's own copies: a second React would break
// hooks, and components from @berth/plugin/ui must be the app's. The app
// publishes them on globalThis.__berth, and the shims in public/shims/
// re-export from there.
//
// A plugin from ~/.berth/plugins is imported only once the user has allowed
// it, and only when the bytes fetched hash to what they allowed (see
// consent.ts): the hashed bytes are the ones imported, so the plugin cannot
// change between the check and the import.
//
// A plugin's source is fetched with the agent's token and imported from a
// blob: URL. Bare imports are rewritten to the shims' absolute URLs before
// that, so loading does not depend on the import map in index.html reaching
// blob: modules (it does in Chromium, and it is the fallback for plugins
// loaded any other way).

const SHIMS: Record<string, string> = {
  react: "react.js",
  "react/jsx-runtime": "react-jsx-runtime.js",
  "react/jsx-dev-runtime": "react-jsx-runtime.js",
  "react-dom": "react-dom.js",
  "@berth/plugin": "berth-plugin.js",
  "@berth/plugin/ui": "berth-plugin-ui.js",
};

export function installGlobals() {
  (globalThis as Record<string, unknown>).__berth = { React, ReactDOM, ReactJSXRuntime, sdk, ui: pluginUi };
}

// rewriteImports points every shared bare import at its shim. It handles
// `from "x"`, `import "x"` and `import("x")`, which is what bundlers emit.
export function rewriteImports(source: string, base = location.href): string {
  return source.replace(/(\bfrom\s*|\bimport\s*\(?\s*)(["'])([^"']+)\2/g, (whole, lead: string, quote: string, spec: string) => {
    const shim = SHIMS[spec];
    return shim ? `${lead}${quote}${new URL(`${import.meta.env.BASE_URL}shims/${shim}`, base).href}${quote}` : whole;
  });
}

const loaded = new Map<string, { dispose?: () => void; url: string }>();

// Built-in plugins ship with the app in public/builtin-plugins/, listed in
// its index.json by `pnpm build:plugins`. A missing index means none. The
// app fetches them from its own origin, so the release CSP's connect-src
// must allow 'self' (scripts/check-csp.mjs).
export async function builtinPlugins(): Promise<PluginInfo[]> {
  try {
    // From the app's base: "/" in the app, the demo's own folder on the web.
    const res = await fetch(new URL(`${import.meta.env.BASE_URL}builtin-plugins/index.json`, location.href));
    if (!res.ok) return [];
    const list = (await res.json()) as PluginInfo[];
    return list.map((p) => ({ ...p, builtin: true, entry: `${import.meta.env.BASE_URL}builtin-plugins/${p.id}/${p.main}` }));
  } catch (err) {
    console.warn("could not list the built-in plugins", err);
    return [];
  }
}

// PluginNeedsReview is thrown for a plugin whose files are not the ones the
// user allowed.
class PluginNeedsReview extends Error {}

// pluginSource is the code to import for p, or null for a plugin with no
// main module (one that only adds hooks, themes, kits or agent tools).
async function pluginSource(client: Client, p: PluginInfo): Promise<string | null> {
  if (p.builtin) {
    const res = await fetch(new URL(p.entry!, location.href));
    if (!res.ok) throw new Error(`${p.id}: ${res.status}`);
    return res.text();
  }
  const files = await readPluginFiles(client, p);
  if (!mayLoad(p, files.hash)) throw new PluginNeedsReview("It changed since you allowed it. Review it to turn it back on.");
  if (!files.mainPath) return null;
  return new TextDecoder().decode(files.main);
}

async function loadPlugin(client: Client, p: PluginInfo) {
  setPluginStatus({ id: p.id, name: p.name, version: p.version, state: "loading", builtin: p.builtin });
  try {
    if (p.error) throw new Error(p.error);
    const code = await pluginSource(client, p);
    if (code === null) {
      // Nothing runs in the app: its hooks and tools run on the boxes.
      setPluginStatus({ id: p.id, name: p.name, version: p.version, state: "active", builtin: p.builtin });
      return;
    }
    const source = rewriteImports(code);
    const url = URL.createObjectURL(new Blob([source], { type: "text/javascript" }));
    const mod = (await import(/* @vite-ignore */ url)) as { default?: unknown };
    if (typeof mod.default !== "function") throw new Error("its module has no default export function");
    const ctx = makeContext(p.id, client);
    pluginContexts.set(p.id, ctx);
    const dispose = await (mod.default as (c: typeof ctx) => unknown)(ctx);
    loaded.set(p.id, { url, dispose: typeof dispose === "function" ? (dispose as () => void) : undefined });
    setPluginStatus({ id: p.id, name: p.name, version: p.version, state: "active", builtin: p.builtin });
  } catch (err) {
    removePlugin(p.id);
    const review = err instanceof PluginNeedsReview;
    setPluginStatus({ id: p.id, name: p.name, version: p.version, state: review ? "review" : "failed", error: plainError(err), builtin: p.builtin });
    if (!review) console.error(`plugin ${p.id} failed to load`, err);
  }
}

export function unloadPlugin(id: string) {
  const l = loaded.get(id);
  try {
    l?.dispose?.();
  } catch (err) {
    console.error(`plugin ${id}: cleanup failed`, err);
  }
  if (l) URL.revokeObjectURL(l.url);
  loaded.delete(id);
  pluginContexts.delete(id);
  removePlugin(id);
}

// loadPlugins (re)loads the built-in plugins that are on, and every enabled
// plugin the agent lists. A user plugin replaces a built-in with its id.
export async function loadPlugins(client: Client) {
  usePluginsLoading.setState(true, true);
  const [user, builtins] = await Promise.all([
    client.plugins().catch((err) => {
      console.warn("could not list plugins", err);
      return [] as PluginInfo[];
    }),
    builtinPlugins(),
  ]);
  for (const id of [...loaded.keys()]) unloadPlugin(id);
  const userIds = new Set(user.map((p) => p.id));
  // A plugin of yours is never on by default: only once allowed (enabled),
  // or, when it changed since, listed so it can be reviewed again.
  const changed = user.filter((p) => !p.builtin && p.enabled !== true && p.changed);
  const list = [...builtins.filter((b) => !userIds.has(b.id) && builtinOn(b)), ...user.filter((p) => !p.builtin && p.enabled === true)];
  setPluginOrder([...list, ...changed].map((p) => p.id));
  for (const p of changed) {
    setPluginStatus({ id: p.id, name: p.name, version: p.version, state: "review", error: "It changed since you allowed it. Review it to turn it back on." });
  }
  await Promise.all(list.map((p) => loadPlugin(client, p)));
  usePluginsLoading.setState(false, true);
  void reviewChanged(client, [...changed, ...user.filter((p) => p.enabled === true && useRegistry.getState().plugins.some((s) => s.id === p.id && s.state === "review"))]);
}

// reviewChanged asks again about a plugin that changed since the user
// allowed it, once per launch: allowing it loads it, anything else leaves it
// off until they turn it on in Settings → Plugins.
async function reviewChanged(client: Client, plugins: PluginInfo[]) {
  const seen = new Set<string>();
  for (const p of plugins) {
    if (seen.has(p.id) || usePluginConsent.getState().dismissed.includes(p.id)) continue;
    seen.add(p.id);
    if (await askToAllow(p)) {
      await loadPlugins(client);
      return;
    }
  }
}

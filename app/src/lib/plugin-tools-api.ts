// The laptop API for a plugin's tools for agents (fork): its setup and
// settings here and on boxes, and installing one from a release. laptopApi
// (api.ts) includes these, and api.ts re-exports the types.

import type { Client, PluginInfo } from "@/lib/api";

export const pluginToolsApi = {
  // The one-time step a plugin's agent tools need on this computer (macOS
  // permissions, say): whether it is done, and starting it.
  pluginSetup: (c: Client, id: string) => c.laptop<PluginSetupState>("GET", `/v1/plugins/${encodeURIComponent(id)}/setup`),
  startPluginSetup: (c: Client, id: string) => c.laptop<PluginInfo>("POST", `/v1/plugins/${encodeURIComponent(id)}/setup`),
  // Switches a plugin lists for its tools (which Chrome profiles agents may
  // use, say). They are kept outside the reviewed files, so changing one
  // keeps the plugin allowed.
  pluginSettings: (c: Client, id: string) => c.laptop<PluginSettingsState>("GET", `/v1/plugins/${encodeURIComponent(id)}/settings`),
  setPluginSetting: (c: Client, id: string, key: string, value: boolean) =>
    c.laptop<PluginSettingsState>("PUT", `/v1/plugins/${encodeURIComponent(id)}/settings`, { key, value }),
  // A plugin's tools on boxes (internal/agent/boxplugins.go): each online
  // box's state, putting it on one or updating it there, taking it off, and
  // starting its setup on the box's screen.
  installPluginFromURL: (c: Client, url: string, sha256: string, replace: boolean) =>
    c.laptop<{ id: string; name?: string; version?: string; sha256: string; replaced?: boolean }>("POST", "/v1/plugins/install-url", { url, sha256, replace }),
  pluginBoxes: (c: Client, id: string) => c.laptop<PluginBoxes>("GET", `/v1/plugins/${encodeURIComponent(id)}/boxes`),
  installPluginOnBox: (c: Client, id: string, box: string) => c.laptop<PluginOnBox>("POST", `/v1/plugins/${encodeURIComponent(id)}/boxes/${encodeURIComponent(box)}`),
  removePluginFromBox: (c: Client, id: string, box: string) => c.laptop<PluginOnBox>("DELETE", `/v1/plugins/${encodeURIComponent(id)}/boxes/${encodeURIComponent(box)}`),
  // A plugin's settings on one box: that box's own (which of its Chrome
  // profiles its agents may use).
  pluginSettingsOnBox: (c: Client, id: string, box: string) =>
    c.laptop<PluginSettingsState>("GET", `/v1/plugins/${encodeURIComponent(id)}/boxes/${encodeURIComponent(box)}/settings`),
  setPluginSettingOnBox: (c: Client, id: string, box: string, key: string, value: boolean) =>
    c.laptop<PluginSettingsState>("PUT", `/v1/plugins/${encodeURIComponent(id)}/boxes/${encodeURIComponent(box)}/settings`, { key, value }),
  startPluginSetupOnBox: (c: Client, id: string, box: string) => c.laptop<{ started: boolean; locked?: boolean }>("POST", `/v1/plugins/${encodeURIComponent(id)}/boxes/${encodeURIComponent(box)}/setup`),
};

export interface PluginSetting {
  key: string;
  label: string;
  description?: string;
  value: boolean;
  // Shown before the person turns it on.
  warning?: string;
}

export interface PluginSettingsState {
  label?: string;
  settings: PluginSetting[];
}

// A plugin's tools on one box: off (not there), on (as allowed here),
// outdated (another version), unsupported (built for another platform),
// old_berthd (the box can't take plugins yet) or error.
export interface PluginOnBox {
  box: string;
  state: "off" | "on" | "outdated" | "unsupported" | "old_berthd" | "error";
  platform?: string;
  version?: string;
  setup?: PluginSetupState;
  error?: string;
}

export interface PluginBoxes {
  plugin: string;
  tools: boolean;
  enabled: boolean;
  platforms?: string[];
  boxes: PluginOnBox[];
}

export interface PluginSetupState {
  label?: string;
  // Done, or the plugin cannot tell (known is then false).
  ready: boolean;
  known: boolean;
  error?: string;
  // What is left, in the plugin's own words, while it isn't ready.
  problem?: string;
  // On a box: its screen is locked, so setup there waits for someone to
  // unlock it.
  locked?: boolean;
}

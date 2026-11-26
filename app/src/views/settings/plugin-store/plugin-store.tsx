// The fork's Settings → Plugins: laid out like the plugin pages people know
// from ChatGPT and Claude. A search and a filter on top, your plugins as
// compact rows, the listing as a two-column grid of cards, and everything
// long (a plugin's switch, setup, settings, boxes, removal) in a sheet that
// opens from its row or card. The rows inside that sheet are upstream's own
// (plugins-section.tsx's PluginRow and BuiltinRow), so turning a plugin on
// still asks first, and stopping its helper still warns, exactly as before.
import {
  CheckIcon,
  ChevronRightIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
} from "lucide-react";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";

import { ErrorText } from "@/components/error-note";
import { PickOne } from "@/components/pick-one";
import { Tip } from "@/components/tip";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Menu,
  MenuItem,
  MenuPopup,
  MenuSeparator,
  MenuTrigger,
} from "@/components/ui/menu";
import {
  Sheet,
  SheetDescription,
  SheetHeader,
  SheetPanel,
  SheetPopup,
  SheetTitle,
} from "@/components/ui/sheet";
import { toastManager } from "@/components/ui/toast";
import { ApiError, laptopApi, type PluginInfo } from "@/lib/api";
import { plainError } from "@/lib/errors";
import { bytes, errorMessage } from "@/lib/format";
import {
  marketAction,
  marketNote,
  type Marketplace,
  type MarketPlugin,
} from "@/lib/marketplace";
import { openDocs, openUrl } from "@/lib/open-url";
import { APP_NAME } from "@/lib/platform";
import { builtinOn, usePrefs } from "@/lib/prefs";
import { useStore } from "@/lib/store";
import { cn } from "@/lib/utils";
import { askToAllow, usePluginConsent } from "@/plugins/consent";
import { builtinPlugins, loadPlugins } from "@/plugins/host";
import { useRegistry } from "@/plugins/registry";
import { ConfirmDialog } from "@/views/settings/confirm";
import { InstallFromURL } from "@/views/settings/plugin-tools";
import { BuiltinRow, PluginRow } from "@/views/settings/plugins-section";
import { Code, SettingsPage } from "@/views/settings/rows";
import { PluginMark } from "@/views/settings/plugin-store/plugin-icons";

type Filter = "all" | "installed" | "discover" | "builtin";
type Open =
  | { kind: "installed"; id: string }
  | { kind: "listed"; id: string }
  | { kind: "builtin"; id: string };

export function PluginStore() {
  const client = useStore((s) => s.client);
  const loaded = useRegistry((s) => s.plugins);
  const [installed, setInstalled] = useState<PluginInfo[]>();
  const [builtins, setBuiltins] = useState<PluginInfo[]>();
  const [reloading, setReloading] = useState(false);
  const [fromURL, setFromURL] = useState(false);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [open, setOpen] = useState<Open>();
  useEffect(() => void builtinPlugins().then(setBuiltins), []);

  const refresh = useCallback(async () => {
    if (client) setInstalled(await client.plugins().catch(() => []));
  }, [client]);
  // Read again whenever the consent dialog closes: allowing a plugin there
  // turns it on, and it also opens by itself for a plugin an update changed.
  const consenting = usePluginConsent((s) => !!s.request);
  useEffect(() => {
    if (!consenting) void refresh();
  }, [refresh, consenting]);

  const [market, setMarket] = useState<{
    list?: Marketplace;
    error?: string;
    old?: boolean;
    reading?: boolean;
  }>({ reading: true });
  const browse = useCallback(
    async (fresh = false) => {
      if (!client) return;
      setMarket((m) => ({ ...m, reading: true }));
      try {
        setMarket({ list: await laptopApi.marketplace(client, fresh) });
      } catch (err) {
        if (err instanceof ApiError && err.status === 404 && !err.code)
          setMarket({ old: true });
        else setMarket({ error: plainError(err) });
      }
    },
    [client],
  );
  useEffect(() => void browse(), [browse]);

  const reload = useCallback(async () => {
    if (!client) return;
    setReloading(true);
    await loadPlugins(client);
    await Promise.all([refresh(), browse()]);
    setReloading(false);
  }, [client, refresh, browse]);

  const q = query.trim().toLowerCase();
  const matches = (p: {
    id: string;
    name: string;
    description?: string;
    author?: string;
  }) =>
    !q ||
    [p.id, p.name, p.description, p.author].some((s) =>
      s?.toLowerCase().includes(q),
    );
  const listed = market.list?.plugins ?? [];
  const yours = (installed ?? []).filter(matches);
  const discover = listed.filter(matches);
  const shownBuiltins = (builtins ?? []).filter(matches);
  const updates = listed.filter((p) => marketAction(p) === "update");
  const show = (f: Filter) => filter === "all" || filter === f;

  const openPlugin =
    open &&
    (open.kind === "builtin" ? builtins : installed)?.find(
      (p) => p.id === open.id,
    );
  const openListed =
    open?.kind === "listed" ? listed.find((p) => p.id === open.id) : undefined;
  const gone = open && installed !== undefined && !openPlugin && !openListed;
  useEffect(() => {
    if (gone) setOpen(undefined);
  }, [gone]);

  return (
    <SettingsPage
      title="Plugins"
      description="Tools and panels for your agents. Plugins aren't sandboxed, so each stays off until you allow it."
      actions={
        <AddMenu
          reloading={reloading}
          reading={!!market.reading}
          onFromURL={() => setFromURL(true)}
          onReload={() => void reload()}
          onReadListing={() => void browse(true)}
          host={market.list?.host}
        />
      }
    >
      <div className="-mt-4 mb-8 flex flex-wrap items-center gap-3">
        <div className="flex h-8 min-w-48 flex-1 items-center gap-2 rounded-lg border border-input bg-background px-2.5 focus-within:ring-2 focus-within:ring-ring">
          <SearchIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search plugins"
            aria-label="Search plugins"
            data-testid="plugin-search"
            className="min-w-0 flex-1 bg-transparent text-body outline-none placeholder:text-muted-foreground"
          />
        </div>
        <PickOne<Filter>
          label="Show"
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: "All" },
            { value: "installed", label: "Installed" },
            { value: "discover", label: "Discover" },
            { value: "builtin", label: "Built in" },
          ]}
        />
      </div>

      {fromURL && (
        <div className="mb-8">
          <InstallFromURL
            onDone={() => void reload()}
            onClose={() => setFromURL(false)}
          />
        </div>
      )}

      <div className="flex flex-col gap-10">
        {show("installed") && updates.length > 0 && !q && (
          <UpdatesBand
            updates={updates}
            installed={installed}
            onChanged={() => void reload()}
          />
        )}

        {show("installed") && (
          <Section
            title="Your plugins"
            count={installed?.length}
            more={
              filter === "all" && !q && (installed?.length ?? 0) > 0
                ? () => setFilter("installed")
                : undefined
            }
          >
            {installed === undefined ? (
              <Quiet>Loading…</Quiet>
            ) : installed.length === 0 ? (
              <Quiet>
                No plugins yet. Install one from Discover, or{" "}
                <button
                  type="button"
                  onClick={() => void openDocs("/guides/plugins")}
                  className="underline underline-offset-2 hover:text-foreground"
                >
                  write one
                </button>
                .
              </Quiet>
            ) : yours.length === 0 ? (
              <Quiet>None of yours match.</Quiet>
            ) : (
              <div className="divide-y divide-border/60">
                {yours.map((p) => (
                  <InstalledRow
                    key={p.id}
                    plugin={p}
                    listed={listed.find((l) => l.id === p.id)}
                    status={loaded.find((l) => l.id === p.id && !l.builtin)}
                    onOpen={() => setOpen({ kind: "installed", id: p.id })}
                    onChanged={() => void refresh()}
                  />
                ))}
              </div>
            )}
          </Section>
        )}

        {show("discover") && (
          <Section
            title="Discover"
            description={
              market.list ? (
                <>
                  Listed at <Code>{market.list.host}</Code>. Installing one adds
                  it to your plugins, off until you allow it.
                </>
              ) : undefined
            }
            more={
              filter === "all" && !q && listed.length > 6
                ? () => setFilter("discover")
                : undefined
            }
          >
            {market.old ? (
              <Quiet>
                The {APP_NAME} agent on this computer is older than this app, so
                it can't read the listing yet.
              </Quiet>
            ) : market.error ? (
              <div
                className="flex items-center gap-4 py-3.5"
                data-testid="market-error"
              >
                <ErrorText
                  className="min-w-0 flex-1 text-destructive-foreground text-body"
                  text={market.error}
                />
                <Button
                  size="xs"
                  variant="outline"
                  className="min-w-24"
                  loading={market.reading}
                  onClick={() => void browse(true)}
                >
                  Try again
                </Button>
              </div>
            ) : !market.list ? (
              <Quiet>Loading…</Quiet>
            ) : listed.length === 0 ? (
              <Quiet>Nothing is listed yet.</Quiet>
            ) : discover.length === 0 ? (
              <Quiet>Nothing listed matches.</Quiet>
            ) : (
              <div
                className="grid grid-cols-1 gap-2 sm:grid-cols-2"
                data-testid="plugin-grid"
              >
                {(filter === "all" ? discover.slice(0, 6) : discover).map(
                  (p) => (
                    <ListedCard
                      key={p.id}
                      plugin={p}
                      was={installed?.find((u) => u.id === p.id)}
                      onOpen={() =>
                        setOpen(
                          p.installed && !p.by_hand
                            ? { kind: "installed", id: p.id }
                            : { kind: "listed", id: p.id },
                        )
                      }
                      onChanged={() => void reload()}
                      onStale={() => void browse(true)}
                    />
                  ),
                )}
              </div>
            )}
          </Section>
        )}

        {filter === "builtin" ||
        (filter === "all" && q && shownBuiltins.length > 0) ? (
          <Section
            title="Built in"
            description="Ship with the app. One of yours with the same id replaces it."
          >
            {builtins === undefined ? (
              <Quiet>Loading…</Quiet>
            ) : shownBuiltins.length === 0 ? (
              <Quiet>
                {builtins.length
                  ? "None match."
                  : "This build has no built-in plugins."}
              </Quiet>
            ) : (
              <div className="divide-y divide-border/60">
                {shownBuiltins.map((p) => (
                  <BuiltinCompact
                    key={p.id}
                    plugin={p}
                    replaced={installed?.some((u) => u.id === p.id) ?? false}
                    onOpen={() => setOpen({ kind: "builtin", id: p.id })}
                  />
                ))}
              </div>
            )}
          </Section>
        ) : (
          filter === "all" &&
          !q &&
          (builtins?.length ?? 0) > 0 && (
            <button
              type="button"
              onClick={() => setFilter("builtin")}
              className="-mt-4 flex items-center gap-1.5 self-start text-desc text-muted-foreground hover:text-foreground"
              data-testid="plugin-builtins-link"
            >
              {builtins!.length} built-in plugins ship with the app{" "}
              <ChevronRightIcon className="size-3.5" />
            </button>
          )
        )}
      </div>

      <Sheet open={!!open} onOpenChange={(o) => !o && setOpen(undefined)}>
        <SheetPopup className="sm:max-w-lg" data-testid="plugin-sheet">
          {openPlugin && open?.kind === "installed" && (
            <PluginSheet
              plugin={openPlugin}
              listed={listed.find((l) => l.id === openPlugin.id)}
            >
              <PluginRow
                plugin={openPlugin}
                status={loaded.find(
                  (l) => l.id === openPlugin.id && !l.builtin,
                )}
                onChanged={() => void reload()}
              />
            </PluginSheet>
          )}
          {openPlugin && open?.kind === "builtin" && (
            <PluginSheet plugin={openPlugin} builtin>
              {/* As PluginRow is, so a test finds either the same way. */}
              <div data-testid="plugin-row" data-plugin={openPlugin.id}>
                <BuiltinRow
                  plugin={openPlugin}
                  status={loaded.find(
                    (l) => l.id === openPlugin.id && l.builtin,
                  )}
                  replaced={
                    installed?.some((u) => u.id === openPlugin.id) ?? false
                  }
                  onChanged={() => void reload()}
                />
              </div>
            </PluginSheet>
          )}
          {openListed && (
            <ListedSheet
              plugin={openListed}
              was={installed?.find((u) => u.id === openListed.id)}
              onChanged={() => {
                void reload();
                setOpen(undefined);
              }}
              onStale={() => void browse(true)}
            />
          )}
        </SheetPopup>
      </Sheet>
    </SettingsPage>
  );
}

// AddMenu gathers what the page's head used to spread out: installing from
// a release's address, reading plugins and the listing again.
function AddMenu({
  reloading,
  reading,
  onFromURL,
  onReload,
  onReadListing,
  host,
}: {
  reloading: boolean;
  reading: boolean;
  onFromURL(): void;
  onReload(): void;
  onReadListing(): void;
  host?: string;
}) {
  return (
    <Menu>
      <MenuTrigger
        render={
          <Button
            size="sm"
            variant="outline"
            className="gap-1.5"
            loading={reloading || reading}
            data-testid="plugin-add-menu"
          />
        }
      >
        <PlusIcon /> Add
      </MenuTrigger>
      <MenuPopup align="end" className="min-w-56">
        <MenuItem onClick={onFromURL} data-testid="plugin-install-url">
          Install from URL…
        </MenuItem>
        <MenuSeparator />
        <MenuItem onClick={onReload}>
          <RefreshCwIcon /> Reload plugins
        </MenuItem>
        <MenuItem onClick={onReadListing}>
          <RefreshCwIcon /> Read the listing again
        </MenuItem>
        <MenuSeparator />
        <MenuItem onClick={() => void openDocs("/guides/plugins")}>
          Write a plugin…
        </MenuItem>
        {host && (
          <div className="px-2 py-1.5 font-mono text-muted-foreground text-tag">
            Listing: {host}
          </div>
        )}
      </MenuPopup>
    </Menu>
  );
}

function Section({
  title,
  description,
  count,
  more,
  children,
}: {
  title: string;
  description?: ReactNode;
  count?: number;
  more?: () => void;
  children: ReactNode;
}) {
  return (
    <section>
      <div className="mb-2 flex items-end gap-3">
        <div className="min-w-0 flex-1">
          <h2 className="flex items-baseline gap-2 text-foreground text-section">
            {title}
            {count !== undefined && count > 0 && (
              <span className="text-muted-foreground text-meta">{count}</span>
            )}
          </h2>
          {description && (
            <p className="mt-0.5 text-desc text-muted-foreground">
              {description}
            </p>
          )}
        </div>
        {more && (
          <button
            type="button"
            onClick={more}
            className="flex items-center gap-1 text-desc text-muted-foreground hover:text-foreground"
          >
            Show all <ChevronRightIcon className="size-3.5" />
          </button>
        )}
      </div>
      {children}
    </section>
  );
}

function Quiet({ children }: { children: ReactNode }) {
  return (
    <div className="py-6 text-center text-body text-muted-foreground">
      {children}
    </div>
  );
}

type RowState = "on" | "off" | "review" | "failed";

function rowState(
  p: PluginInfo,
  status?: { state: string; error?: string },
): RowState {
  const enabled = p.enabled === true;
  if (!enabled && (p.changed || status?.state === "review")) return "review";
  if (!enabled) return "off";
  if (p.error ?? status?.error) return "failed";
  return "on";
}

function StateChip({ state }: { state: RowState }) {
  const label = {
    on: "On",
    off: "Off",
    review: "Needs review",
    failed: "Failed",
  }[state];
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 text-meta",
        state === "on"
          ? "text-foreground"
          : state === "review"
            ? "text-warning-foreground"
            : state === "failed"
              ? "text-destructive-foreground"
              : "text-muted-foreground",
      )}
    >
      <span
        className={cn(
          "size-1.5 rounded-full",
          state === "on"
            ? "bg-success"
            : state === "review"
              ? "bg-warning"
              : state === "failed"
                ? "bg-destructive"
                : "bg-muted-foreground/40",
        )}
      />
      {label}
    </span>
  );
}

const kindOf = (p: PluginInfo) =>
  p.mcp?.length || p.skills?.length ? "Agent tools" : "Panels and commands";

// InstalledRow is one of your plugins in one line. Its controls are in the
// sheet the row opens; only a review waiting is offered here, since that is
// the one thing it needs from you.
function InstalledRow({
  plugin: p,
  listed,
  status,
  onOpen,
  onChanged,
}: {
  plugin: PluginInfo;
  listed?: MarketPlugin;
  status?: { state: string; error?: string };
  onOpen(): void;
  onChanged(): void;
}) {
  const state = rowState(p, status);
  const update = listed && marketAction(listed) === "update";
  return (
    <div
      className="group flex items-center gap-3 py-2.5"
      data-testid="plugin-item"
      data-plugin={p.id}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-3 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring rounded-lg"
        aria-label={`${p.name}: details`}
      >
        <PluginMark plugin={p} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2 text-row">
            <span className="truncate">{p.name}</span>
            <span className="text-muted-foreground text-meta">{p.version}</span>
            {update && (
              <Badge variant="outline" size="sm">
                {listed!.version} available
              </Badge>
            )}
          </div>
          <div className="truncate text-desc text-muted-foreground">
            {kindOf(p)}
            {p.source === "marketplace" ? " · from the listing" : " · your own"}
          </div>
        </div>
      </button>
      {state === "review" ? (
        <Button
          size="xs"
          variant="outline"
          onClick={() => void askToAllow(p).then((ok) => ok && onChanged())}
        >
          Review
        </Button>
      ) : (
        <StateChip state={state} />
      )}
      <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
    </div>
  );
}

function BuiltinCompact({
  plugin: p,
  replaced,
  onOpen,
}: {
  plugin: PluginInfo;
  replaced: boolean;
  onOpen(): void;
}) {
  const off = usePrefs((s) => !builtinOn(p, s));
  return (
    <div
      className="flex items-center gap-3 py-2.5"
      data-testid="plugin-item"
      data-plugin={p.id}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-3 rounded-lg text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`${p.name}: details`}
      >
        <PluginMark plugin={p} builtin />
        <div className="min-w-0 flex-1">
          <div className="text-row">{p.name}</div>
          <div className="truncate text-desc text-muted-foreground">
            {replaced
              ? "Replaced by your plugin with the same id"
              : p.description}
          </div>
        </div>
      </button>
      <StateChip state={replaced || off ? "off" : "on"} />
      <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
    </div>
  );
}

// useInstall installs or updates a listed plugin, as Browse's rows did.
function useInstall(
  p: MarketPlugin,
  was: PluginInfo | undefined,
  onChanged: () => void,
  onStale: () => void,
) {
  const [busy, setBusy] = useState(false);
  const action = marketAction(p);
  const install = async (replace = false) => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy(true);
    try {
      await laptopApi.installPlugin(client, p.id, p.version, replace);
      toastManager.add({
        title:
          action === "update"
            ? `Updated ${p.name} to ${p.version}`
            : `Installed ${p.name}`,
        description: was?.allowed
          ? `${APP_NAME} asks again before the new code runs.`
          : "It's off until you turn it on.",
        type: "success",
      });
      onChanged();
    } catch (err) {
      toastManager.add({
        title: `Couldn't ${action === "update" ? "update" : "install"} ${p.name}`,
        description: errorMessage(err),
        type: "error",
      });
      if (
        err instanceof ApiError &&
        (err.code === "listing_changed" || err.code === "plugin_by_hand")
      )
        onStale();
    } finally {
      setBusy(false);
    }
  };
  return { busy, action, install };
}

function ListedAction({
  plugin: p,
  busy,
  action,
  onInstall,
  onReplace,
}: {
  plugin: MarketPlugin;
  busy: boolean;
  action: ReturnType<typeof marketAction>;
  onInstall(): void;
  onReplace(): void;
}) {
  if (action === "installed")
    return (
      <Tip label="Installed">
        <span
          className="flex size-7 shrink-0 items-center justify-center text-muted-foreground"
          aria-label="Installed"
        >
          <CheckIcon className="size-4" />
          <span className="sr-only">Installed</span>
        </span>
      </Tip>
    );
  if (action === "update")
    return (
      <Button size="xs" variant="outline" loading={busy} onClick={onInstall}>
        Update
      </Button>
    );
  if (action === "replace")
    return (
      <Button size="xs" variant="outline" loading={busy} onClick={onReplace}>
        Replace…
      </Button>
    );
  if (action === "install")
    return (
      <Tip label={`Install ${p.name}`}>
        <Button
          size="icon-xs"
          variant="outline"
          loading={busy}
          onClick={onInstall}
          aria-label="Install"
        >
          <PlusIcon />
        </Button>
      </Tip>
    );
  return null;
}

// ListedCard is one plugin of the listing, as a card of the grid.
function ListedCard({
  plugin: p,
  was,
  onOpen,
  onChanged,
  onStale,
}: {
  plugin: MarketPlugin;
  was?: PluginInfo;
  onOpen(): void;
  onChanged(): void;
  onStale(): void;
}) {
  const { busy, action, install } = useInstall(p, was, onChanged, onStale);
  const [replacing, setReplacing] = useState(false);
  const note = marketNote(p);
  return (
    <div
      className={cn(
        "group flex items-start gap-3 rounded-xl border border-transparent p-3 hover:border-border hover:bg-accent/40",
        action === "unavailable" && "opacity-60",
      )}
      data-testid="market-plugin"
      data-plugin={p.id}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-start gap-3 text-left outline-none"
        aria-label={`${p.name}: details`}
      >
        <PluginMark plugin={p} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-1.5 text-row">
            <span className="truncate">{p.name}</span>
            <span className="text-muted-foreground text-meta">{p.version}</span>
          </div>
          {p.description && (
            <div className="line-clamp-2 text-desc text-muted-foreground">
              {p.description}
            </div>
          )}
          <div className="mt-0.5 truncate text-muted-foreground text-tag">
            {[p.author && `by ${p.author}`, bytes(p.size)]
              .filter(Boolean)
              .join(" · ")}
          </div>
          {note && (
            <div className="mt-0.5 line-clamp-2 text-meta text-muted-foreground">
              {note}
            </div>
          )}
          {p.error && (
            <div className="mt-0.5 line-clamp-2 text-destructive-foreground text-meta">
              It can't be installed: {p.error}.
            </div>
          )}
        </div>
      </button>
      <ListedAction
        plugin={p}
        busy={busy}
        action={action}
        onInstall={() => void install()}
        onReplace={() => setReplacing(true)}
      />
      <ReplaceDialog
        plugin={p}
        open={replacing}
        onOpenChange={setReplacing}
        onConfirm={() => install(true)}
      />
    </div>
  );
}

function ReplaceDialog({
  plugin: p,
  open,
  onOpenChange,
  onConfirm,
}: {
  plugin: MarketPlugin;
  open: boolean;
  onOpenChange(o: boolean): void;
  onConfirm(): Promise<void>;
}) {
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      destructive
      title={`Replace your ${p.name}?`}
      description={`~/.berth/plugins/${p.id} holds a plugin the listing didn't install, or one changed since. Its folder is deleted and ${p.name} ${p.version} from the listing takes its place, off until you allow it.`}
      confirm="Replace"
      onConfirm={onConfirm}
    />
  );
}

// UpdatesBand says what the listing has newer, with one button for all.
function UpdatesBand({
  updates,
  installed,
  onChanged,
}: {
  updates: MarketPlugin[];
  installed?: PluginInfo[];
  onChanged(): void;
}) {
  const [busy, setBusy] = useState(false);
  const updateAll = async () => {
    const client = useStore.getState().client;
    if (!client) return;
    setBusy(true);
    const failed: string[] = [];
    for (const p of updates) {
      try {
        await laptopApi.installPlugin(client, p.id, p.version);
      } catch {
        failed.push(p.name);
      }
    }
    setBusy(false);
    const asked = updates.some(
      (p) => installed?.find((u) => u.id === p.id)?.allowed,
    );
    toastManager.add(
      failed.length
        ? {
            title: `Couldn't update ${failed.join(", ")}`,
            description: "The others were updated.",
            type: "error",
          }
        : {
            title: `Updated ${updates.length === 1 ? updates[0].name : `${updates.length} plugins`}`,
            description: asked
              ? `${APP_NAME} asks again before new code runs.`
              : undefined,
            type: "success",
          },
    );
    onChanged();
  };
  return (
    <div
      className="flex items-center gap-3 rounded-xl border border-border bg-accent/30 px-3 py-2.5"
      data-testid="plugin-updates"
    >
      <span className="size-2 shrink-0 rounded-full bg-info" />
      <div className="min-w-0 flex-1 text-body">
        {updates.length === 1
          ? `${updates[0].name} ${updates[0].version} is available`
          : `${updates.length} updates available`}
        <span className="text-muted-foreground">
          {" "}
          ·{" "}
          {updates
            .map((p) => `${p.name} ${p.installed_version ?? ""} → ${p.version}`)
            .join(", ")}
        </span>
      </div>
      <Button
        size="xs"
        variant="outline"
        loading={busy}
        onClick={() => void updateAll()}
      >
        {updates.length === 1 ? "Update" : "Update all"}
      </Button>
    </div>
  );
}

// PluginSheet is an installed (or built-in) plugin's own page: its name and
// what it is on top, then upstream's row with every control it has.
function PluginSheet({
  plugin: p,
  listed,
  builtin,
  children,
}: {
  plugin: PluginInfo;
  listed?: MarketPlugin;
  builtin?: boolean;
  children: ReactNode;
}) {
  const meta = useMemo(
    () =>
      [
        builtin
          ? "Built in"
          : p.source === "marketplace"
            ? "From the listing"
            : "Your own",
        kindOf(p),
        listed?.author && `by ${listed.author}`,
      ]
        .filter(Boolean)
        .join(" · "),
    [p, listed, builtin],
  );
  return (
    <>
      <SheetHeader>
        <div className="flex items-center gap-3">
          <PluginMark plugin={p} builtin={builtin} size="lg" />
          <div className="min-w-0">
            <SheetTitle>
              {p.name}{" "}
              <span className="font-normal text-muted-foreground text-meta">
                {p.version}
              </span>
            </SheetTitle>
            <SheetDescription>{meta}</SheetDescription>
          </div>
        </div>
      </SheetHeader>
      <SheetPanel className="flex flex-col gap-2 [&_[data-testid=plugin-row]>div:first-child>div:first-child]:hidden">
        {listed && marketAction(listed) === "update" && (
          <div className="text-desc text-muted-foreground">
            {listed.version} is in the listing: update it from Your plugins or
            Discover.
          </div>
        )}
        {children}
        {!builtin && (
          <div className="pt-2 font-mono text-muted-foreground text-tag">
            ~/.berth/plugins/{p.id}
          </div>
        )}
        {listed?.homepage?.startsWith("https://") && (
          <button
            type="button"
            onClick={() => void openUrl(listed.homepage!)}
            className="self-start text-desc text-muted-foreground underline underline-offset-2 hover:text-foreground"
          >
            Homepage
          </button>
        )}
      </SheetPanel>
    </>
  );
}

// ListedSheet is a listed plugin not installed (or in the way of one of
// yours): what it is and what installing does, before anything runs.
function ListedSheet({
  plugin: p,
  was,
  onChanged,
  onStale,
}: {
  plugin: MarketPlugin;
  was?: PluginInfo;
  onChanged(): void;
  onStale(): void;
}) {
  const { busy, action, install } = useInstall(p, was, onChanged, onStale);
  const [replacing, setReplacing] = useState(false);
  const note = marketNote(p);
  return (
    <>
      <SheetHeader>
        <div className="flex items-center gap-3">
          <PluginMark plugin={p} size="lg" />
          <div className="min-w-0">
            <SheetTitle>
              {p.name}{" "}
              <span className="font-normal text-muted-foreground text-meta">
                {p.version}
              </span>
            </SheetTitle>
            <SheetDescription>
              {[p.author && `by ${p.author}`, bytes(p.size)]
                .filter(Boolean)
                .join(" · ")}
            </SheetDescription>
          </div>
        </div>
      </SheetHeader>
      <SheetPanel className="flex flex-col gap-4">
        {p.description && <p className="text-body">{p.description}</p>}
        <div className="rounded-xl border border-border p-3 text-desc text-muted-foreground">
          Installing puts it in <Code>~/.berth/plugins/{p.id}</Code>, off.
          Turning it on shows what it will be able to do and asks you first.
          Plugins aren't sandboxed.
        </div>
        {note && <div className="text-desc text-muted-foreground">{note}</div>}
        {p.error && (
          <div className="text-desc">
            <span className="text-destructive-foreground">
              It can't be installed:
            </span>{" "}
            <span className="text-muted-foreground">{p.error}.</span>
          </div>
        )}
        <div className="flex items-center gap-2">
          {action === "install" || action === "update" ? (
            <Button size="sm" loading={busy} onClick={() => void install()}>
              {action === "update" ? `Update to ${p.version}` : "Install"}
            </Button>
          ) : action === "replace" ? (
            <Button
              size="sm"
              variant="destructive-outline"
              loading={busy}
              onClick={() => setReplacing(true)}
            >
              Replace yours…
            </Button>
          ) : null}
          {p.homepage?.startsWith("https://") && (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => void openUrl(p.homepage!)}
            >
              Homepage
            </Button>
          )}
        </div>
        <div className="font-mono text-muted-foreground text-tag">{p.id}</div>
      </SheetPanel>
      <ReplaceDialog
        plugin={p}
        open={replacing}
        onOpenChange={setReplacing}
        onConfirm={() => install(true)}
      />
    </>
  );
}

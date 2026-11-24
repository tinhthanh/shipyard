import { CheckIcon, ChevronsUpDownIcon, PinIcon, PinOffIcon, SearchIcon } from "lucide-react";
import { type ReactNode, useMemo, useState } from "react";
import { create } from "zustand";

import type { Action } from "@/components/sidebar/actions";
import { switchTo } from "@/components/sidebar/columns";
import { ink } from "@/components/sidebar/ink";
import { Tip } from "@/components/tip";
import { Popover, PopoverPopup, PopoverTrigger } from "@/components/ui/popover";
import { usePrefs } from "@/lib/prefs";
import { type Project, useProjects } from "@/lib/project-groups";
import { load, save } from "@/lib/storage";
import { cn } from "@/lib/utils";

// Pinned projects (the fork's): once you pin one, the rail shows only the
// projects you pin, and the one you are in; until then it shows them all,
// as it always has. Every project is a click away in the switcher on the
// sheet's heading, which pins and unpins them too. Pins are this device's,
// kept in localStorage. rail-layout.tsx and actions.tsx call in.

const KEY = "berth.projectPins";

// null: nothing pinned yet, so the rail shows every project.
export const useProjectPins = create<{ ids: string[] | null }>(() => ({ ids: load<string[] | null>(KEY, null) }));
useProjectPins.subscribe((s) => save(KEY, s.ids));

export const isPinned = (id: string) => !!useProjectPins.getState().ids?.includes(id);

export function setPinned(id: string, on: boolean) {
  const ids = (useProjectPins.getState().ids ?? []).filter((x) => x !== id);
  useProjectPins.setState({ ids: on ? [...ids, id] : ids });
}

// Every project on the rail again, as before any was pinned.
export const clearPins = () => useProjectPins.setState({ ids: null });

// useRailProjects is what the rail lists: the pinned projects in the
// sidebar's order, and the picked one even when it isn't pinned, so you can
// see where you are.
export function useRailProjects(projects: Project[], picked?: string): Project[] {
  const ids = useProjectPins((s) => s.ids);
  return useMemo(() => (ids ? projects.filter((p) => p.id === picked || ids.includes(p.id)) : projects), [projects, ids, picked]);
}

// The project's menu item that pins it to the rail or takes it off.
export function pinAction(p: Project): Action {
  const on = isPinned(p.id);
  return { type: "item", label: on ? "Unpin from the rail" : "Pin to the rail", icon: on ? <PinOffIcon /> : <PinIcon />, run: () => setPinned(p.id, !on) };
}

// pickProject opens a project on the sheet, as its rail tile does.
export function pickProject(p: Project) {
  usePrefs.setState({ sidebarPick: p.id, tasksFolded: false });
  switchTo(p.members, p.members.find((m) => m.box.name === p.defaultBox) ?? p.members[0]);
}

// The projects as the sidebar orders them: loose ones, then each section's.
function useOrdered() {
  const { projects, sections } = useProjects();
  return useMemo(() => [...projects.filter((p) => !p.section), ...sections.flatMap((name) => projects.filter((p) => p.section === name))], [projects, sections]);
}

// ProjectSwitcher makes the sheet's heading (children) open a list of every
// project: click one to go there, the pin beside it to keep it on the rail.
export function ProjectSwitcher({ current, avatar, children }: { current: Project; avatar: (p: Project) => ReactNode; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const ordered = useOrdered();
  const all = useProjectPins((s) => s.ids);
  const ids = all ?? [];
  const q = query.trim().toLowerCase();
  const shown = ordered.filter((p) => !q || p.name.toLowerCase().includes(q) || p.members.some((m) => m.box.name.toLowerCase().includes(q)));
  const pinned = shown.filter((p) => ids.includes(p.id));
  const rest = shown.filter((p) => !ids.includes(p.id));
  const go = (p: Project) => {
    setOpen(false);
    setQuery("");
    if (p.id !== current.id) pickProject(p);
  };

  const row = (p: Project) => {
    const on = ids.includes(p.id);
    const here = p.id === current.id;
    return (
      <li key={p.id} className="group/pick flex items-center gap-1 rounded-md hover:bg-accent">
        <button
          type="button"
          data-testid="switcher-project"
          data-project={p.name}
          aria-current={here ? "true" : undefined}
          onClick={() => go(p)}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {avatar(p)}
          <span className="flex min-w-0 flex-1 flex-col">
            <span className={cn("truncate text-body", ink.strong)}>{p.name}</span>
            <span className={cn("truncate text-tag", ink.faint)}>{p.members.map((m) => m.box.name).join(" · ")}</span>
          </span>
          {here && <CheckIcon className="size-3.5 shrink-0 text-muted-foreground" />}
        </button>
        <Tip label={on ? "Unpin from the rail" : "Pin to the rail"} side="right">
          <button
            type="button"
            data-testid="switcher-pin"
            aria-label={on ? `Unpin ${p.name} from the rail` : `Pin ${p.name} to the rail`}
            aria-pressed={on}
            onClick={() => setPinned(p.id, !on)}
            className={cn(
              "mr-1 inline-flex size-7 shrink-0 items-center justify-center rounded-md hover:bg-background hover:text-foreground [&_svg]:size-3.5",
              on ? "text-foreground" : "text-muted-foreground opacity-0 group-hover/pick:opacity-100 focus-visible:opacity-100",
            )}
          >
            {on ? <PinIcon className="fill-current" /> : <PinIcon />}
          </button>
        </Tip>
      </li>
    );
  };

  return (
    <Popover
      open={open}
      onOpenChange={(o: boolean) => {
        setOpen(o);
        if (!o) setQuery("");
      }}
    >
      <PopoverTrigger
        data-testid="project-switcher"
        aria-label={`${current.name}: switch project`}
        className="-my-1 -mx-1.5 flex min-w-0 flex-1 items-center gap-1.5 rounded-lg px-1.5 py-1 text-left outline-none hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-ring data-popup-open:bg-sidebar-accent"
      >
        {children}
        <ChevronsUpDownIcon className={cn("size-3.5 shrink-0", ink.glyph)} />
      </PopoverTrigger>
      <PopoverPopup side="bottom" align="start" className="w-80 p-0" data-testid="project-switcher-popup">
        <div className="flex items-center gap-2 border-b px-3 py-2">
          <SearchIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && shown[0]) go(shown[0]);
            }}
            placeholder="Find a project…"
            aria-label="Find a project"
            className="min-w-0 flex-1 bg-transparent text-body outline-none placeholder:text-muted-foreground"
          />
        </div>
        <div className="max-h-[min(60vh,28rem)] overflow-y-auto p-1">
          {pinned.length > 0 && (
            <>
              <p className={cn("px-2 pt-1.5 pb-1 text-label", ink.faint)}>Pinned</p>
              <ul>{pinned.map(row)}</ul>
            </>
          )}
          {rest.length > 0 && (
            <>
              <p className={cn("px-2 pt-1.5 pb-1 text-label", ink.faint)}>{pinned.length ? "Other projects" : "Projects"}</p>
              <ul>{rest.map(row)}</ul>
            </>
          )}
          {shown.length === 0 && <p className={cn("px-2 py-3 text-center text-desc", ink.faint)}>No project matches.</p>}
        </div>
        <div className={cn("flex items-center gap-2 border-t px-3 py-2 text-tag", ink.faint)}>
          <span className="flex-1">{all ? "Only pinned projects, and the one you are in, show on the rail." : "Pin a project to show only pinned ones on the rail."}</span>
          {all && (
            <button type="button" data-testid="switcher-show-all" onClick={clearPins} className="shrink-0 rounded px-1 underline-offset-2 hover:text-foreground hover:underline">
              Show all
            </button>
          )}
        </div>
      </PopoverPopup>
    </Popover>
  );
}

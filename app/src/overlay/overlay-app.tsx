import { invoke } from "@tauri-apps/api/core";
import { ShipIcon } from "lucide-react";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";

import type { LoginLimits } from "../../../plugins/usage/src/summary.ts";
import { AgentIcon } from "@/components/agent-glyph";
import { isTauri } from "@/lib/api";
import type { OverlayPlace } from "@/lib/prefs";
import { cn } from "@/lib/utils";
import { type OverlayTarget, OVERLAY_OPEN } from "@/overlay/control";
import { useLoginLimits, useOverlayConnection, usageOn } from "@/overlay/data";
import { type AgentRow, useAgentRows } from "@/views/home/widgets/agents";

// The floating bar's page (index.html?window=overlay&place=edge|notch): a
// pill on the screen's right edge, or a black band around the notch, with a
// ring for each login's plan limits and the agents that need you or are
// working. Hovering opens a card with the detail; a click opens it in the
// main window. Its window is sized to it (overlay.rs, overlay_fit).

interface Screen {
  width: number;
  height: number;
  notch: { width: number; height: number } | null;
  top: number;
}

const place = (): OverlayPlace => (new URLSearchParams(location.search).get("place") === "notch" ? "notch" : "edge");

// In a browser (mock mode, tests) there is no window to fit: the page shows
// what the window would, at a made-up screen's size.
async function readScreen(p: OverlayPlace): Promise<Screen> {
  if (isTauri()) return invoke<Screen>("overlay_screen");
  return { width: window.innerWidth, height: window.innerHeight, notch: p === "notch" ? { width: 180, height: 32 } : null, top: 32 };
}

function open(target: OverlayTarget | null) {
  if (isTauri()) void invoke("overlay_open", { target });
  else window.dispatchEvent(new CustomEvent(OVERLAY_OPEN, { detail: target }));
}

export default function OverlayApp() {
  useOverlayConnection();
  const where = place();
  const [screen, setScreen] = useState<Screen | null>(null);
  useEffect(() => void readScreen(where).then(setScreen), [where]);
  const [on] = useState(usageOn);
  const limits = useLoginLimits(on);
  const rows = useAgentRows();
  const waiting = useMemo(() => rows.filter((r) => r.state === "waiting"), [rows]);
  const working = useMemo(() => rows.filter((r) => r.state === "running"), [rows]);
  const [hover, setHover] = useHover();

  // The page is the window: nothing behind it.
  useLayoutEffect(() => {
    document.documentElement.style.background = "transparent";
    document.body.style.background = "transparent";
    document.documentElement.classList.add("dark");
  }, []);

  const root = useRef<HTMLDivElement>(null);
  useFit(root, where);

  if (!screen) return null;
  const notch = where === "notch";
  const bar = (
    <Bar notch={notch} screen={screen}>
      <Rings limits={limits} small={notch && !screen.notch} />
      <Counts waiting={waiting} working={working} />
    </Bar>
  );
  return (
    <div
      ref={root}
      data-testid="overlay"
      data-place={where}
      className={cn("inline-flex select-none text-neutral-100", notch ? "flex-col items-center" : "flex-row-reverse items-center gap-2")}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      {bar}
      {hover && <Card limits={limits} usage={on} waiting={waiting} working={working} />}
    </div>
  );
}

// The bar itself. At the notch it is as tall as the notch and leaves its
// width empty in the middle, the rings on its left and the agents on its
// right; on the edge, a pill top to bottom.
function Bar({ notch, screen, children }: { notch: boolean; screen: Screen; children: [React.ReactNode, React.ReactNode] }) {
  if (!notch) {
    return (
      <div className="flex w-11 flex-col items-center gap-2 rounded-full border border-white/10 bg-black/85 px-1 py-2.5 shadow-lg backdrop-blur" data-testid="overlay-bar">
        {children[0]}
        {children[1]}
      </div>
    );
  }
  const height = Math.max(24, Math.round(screen.notch?.height ?? screen.top ?? 24));
  return (
    // Two equal columns either side of the gap, so the gap stays over the
    // notch however much each side shows.
    <div className="grid grid-cols-[1fr_auto_1fr] items-center rounded-b-2xl bg-black px-3" style={{ height }} data-testid="overlay-bar">
      <div className="flex items-center justify-end gap-1.5">{children[0]}</div>
      <div style={{ width: screen.notch ? screen.notch.width : 12 }} />
      <div className="flex items-center justify-start gap-2">{children[1]}</div>
    </div>
  );
}

// A ring for each login: the outer arc its shortest window (Claude Code's
// session), the inner its most used other one (a week).
function Rings({ limits, small }: { limits: LoginLimits[]; small: boolean }) {
  if (!limits.length) return <ShipIcon className="size-4 text-neutral-400" aria-label="Shipyard" />;
  return (
    <>
      {limits.map((l) => (
        <button key={`${l.agent}:${l.login}`} type="button" className="rounded-full outline-none focus-visible:ring-2 focus-visible:ring-white/60" aria-label={`${l.name}: ${l.windows.map((w) => `${w.name} ${pct(w.used)}`).join(", ")}`} onClick={() => open(null)}>
          <Ring login={l} size={small ? 20 : 28} />
        </button>
      ))}
    </>
  );
}

function Ring({ login, size }: { login: LoginLimits; size: number }) {
  const [first, ...rest] = login.windows;
  const second = rest.reduce<LoginLimits["windows"][number] | undefined>((m, w) => (!m || w.used > m.used ? w : m), undefined);
  const c = size / 2;
  const arc = (r: number, used: number, stroke: string) => {
    const len = 2 * Math.PI * r;
    return (
      <>
        <circle cx={c} cy={c} r={r} fill="none" stroke="currentColor" strokeOpacity={0.15} strokeWidth={size / 10} />
        <circle cx={c} cy={c} r={r} fill="none" stroke={stroke} strokeWidth={size / 10} strokeLinecap="round" strokeDasharray={`${Math.min(1, used) * len} ${len}`} transform={`rotate(-90 ${c} ${c})`} />
      </>
    );
  };
  return (
    <span className="relative grid place-items-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="absolute inset-0 text-white" aria-hidden>
        {first && arc(c - size / 20 - 0.5, first.used, tone(first.used))}
        {second && arc(c - size / 20 - size / 6.5, second.used, tone(second.used))}
      </svg>
      <AgentIcon agent={login.agent} className={size < 24 ? "size-2" : "size-2.5"} />
    </span>
  );
}

// Amber at 70%, red at 90%: the plan's own warning colours.
const tone = (used: number) => (used >= 0.9 ? "#f87171" : used >= 0.7 ? "#fbbf24" : "#e5e5e5");
const pct = (used: number) => `${Math.round(used * 100)}%`;

function Counts({ waiting, working }: { waiting: AgentRow[]; working: AgentRow[] }) {
  return (
    <>
      {waiting.length > 0 && (
        <button type="button" data-testid="overlay-waiting" className="flex items-center gap-1 rounded-full px-1 font-semibold text-[11px] text-amber-300 tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-white/60" aria-label={`${waiting.length} need you`} onClick={() => open(target(waiting[0]))}>
          <span className="size-2 animate-pulse rounded-full bg-amber-400" />
          {waiting.length}
        </button>
      )}
      {working.length > 0 && (
        <button type="button" data-testid="overlay-working" className="flex items-center gap-1 rounded-full px-1 font-semibold text-[11px] text-sky-300 tabular-nums outline-none focus-visible:ring-2 focus-visible:ring-white/60" aria-label={`${working.length} working`} onClick={() => open(target(working[0]))}>
          <span className="size-2.5 animate-spin rounded-full border-[1.5px] border-sky-400 border-t-transparent motion-reduce:animate-none" />
          {working.length}
        </button>
      )}
    </>
  );
}

const target = (r: AgentRow): OverlayTarget => ({ box: r.box, session: r.session.name });

// The card under the pointer: each login's windows with when they reset,
// and the agents that need you or are working, a click from opening.
function Card({ limits, usage, waiting, working }: { limits: LoginLimits[]; usage: boolean; waiting: AgentRow[]; working: AgentRow[] }) {
  const now = Date.now();
  return (
    <div className="w-72 space-y-3 rounded-xl border border-white/10 bg-neutral-900/95 p-3 text-xs shadow-xl backdrop-blur" data-testid="overlay-card">
      {limits.length > 0 ? (
        limits.map((l) => (
          <section key={`${l.agent}:${l.login}`} className="space-y-1.5">
            <h3 className="flex items-center gap-1.5 font-medium text-neutral-200">
              <AgentIcon agent={l.agent} className="size-3" />
              <span className="truncate">{l.name}</span>
            </h3>
            {l.windows.map((w) => (
              <div key={w.name} className="space-y-0.5">
                <div className="flex items-baseline gap-2 text-[11px]">
                  <span className="min-w-0 flex-1 truncate">{w.name}</span>
                  <span className="shrink-0 text-neutral-400">resets {resetsIn(w.resets - now)}</span>
                  <span className="w-8 shrink-0 text-right tabular-nums">{pct(w.used)}</span>
                </div>
                <span className="block h-1 overflow-hidden rounded-full bg-white/10">
                  <span className="block h-full rounded-full" style={{ width: `${Math.min(100, w.used * 100)}%`, background: tone(w.used) }} />
                </span>
              </div>
            ))}
          </section>
        ))
      ) : (
        <p className="text-neutral-400">{usage ? "No plan limits read yet." : "Turn on the Usage & accounts plugin to see plan limits here."}</p>
      )}
      <Agents title="Need you" rows={waiting} dot="bg-amber-400" />
      <Agents title="Working" rows={working} dot="bg-sky-400" />
      {!waiting.length && !working.length && <p className="text-neutral-400">No agent is working.</p>}
    </div>
  );
}

function Agents({ title, rows, dot }: { title: string; rows: AgentRow[]; dot: string }) {
  if (!rows.length) return null;
  return (
    <section className="space-y-1 border-white/10 border-t pt-2">
      <h3 className="font-medium text-neutral-400">
        {title} · {rows.length}
      </h3>
      {rows.slice(0, 5).map((r) => (
        <button key={r.key} type="button" className="flex w-full items-center gap-2 rounded-md px-1 py-0.5 text-left hover:bg-white/10" onClick={() => open(target(r))}>
          <span className={cn("size-1.5 shrink-0 rounded-full", dot)} />
          <AgentIcon agent={r.agent} className="size-3" />
          <span className="min-w-0 flex-1 truncate">{r.title}</span>
          <span className="max-w-24 shrink-0 truncate text-[10px] text-neutral-500">{r.where}</span>
        </button>
      ))}
      {rows.length > 5 && <p className="px-1 text-[10px] text-neutral-500">+{rows.length - 5} more</p>}
    </section>
  );
}

// "in 42m", "in 3h 5m", "in 2d 4h"
export function resetsIn(ms: number): string {
  const m = Math.max(0, Math.round(ms / 60_000));
  if (m < 60) return `in ${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `in ${h}h${m % 60 ? ` ${m % 60}m` : ""}`;
  return `in ${Math.floor(h / 24)}d${h % 24 ? ` ${h % 24}h` : ""}`;
}

// Hover with a short grace on the way out, so moving from the bar to the
// card (or across its edge) doesn't close it.
function useHover(): [boolean, (on: boolean) => void] {
  const [hover, setHover] = useState(false);
  const timer = useRef(0);
  return [
    hover,
    (on) => {
      window.clearTimeout(timer.current);
      if (on) setHover(true);
      else timer.current = window.setTimeout(() => setHover(false), 250);
    },
  ];
}

// useFit sizes the window to the page whenever what it shows changes size.
function useFit(root: React.RefObject<HTMLDivElement | null>, where: OverlayPlace) {
  useEffect(() => {
    const el = root.current;
    if (!el || !isTauri()) return;
    let last = "";
    const fit = () => {
      const r = el.getBoundingClientRect();
      const width = Math.ceil(r.width);
      const height = Math.ceil(r.height);
      if (!width || !height || `${width}x${height}` === last) return;
      last = `${width}x${height}`;
      void invoke("overlay_fit", { width, height, place: where }).catch((err) => console.warn("floating bar:", err));
    };
    const ro = new ResizeObserver(fit);
    ro.observe(el);
    fit();
    return () => ro.disconnect();
  });
}

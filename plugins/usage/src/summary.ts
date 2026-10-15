import type { Agent, Report, Window } from "./box";
import { AGENT_NAME, days } from "./data.ts";

// What the Usage widget on Home shows, added up from the boxes' reports.
// Kept apart from the widget so it can be tested without a renderer.

export interface UsageSummary {
  today: number;
  // Tokens a day, the last 7 days, oldest first.
  week: { day: string; tokens: number }[];
  // Claude Code's own estimate at API prices, for sessions active this week.
  cost: number;
  byAgent: { agent: Agent; tokens: number }[];
  limits: { label: string; used: number; resets: number }[];
  failed: string[];
}

// A window's short name: "5h", "week", or "week (Fable)" for one scoped to
// a model.
export const windowName = (w: Window) => {
  const h = w.window_minutes / 60;
  const base = h >= 24 * 6 ? "week" : h >= 20 ? "day" : `${h}h`;
  const scope = w.label && /\(([^)]+)\)$/.exec(w.label)?.[1];
  return scope ? `${base} (${scope})` : base;
};

// Two reads of one window whose resets are this close are the same window.
const SAME_RESET_MS = 2 * 60_000;

// summarizeUsage adds up the boxes' reports for the widget.
export function summarizeUsage(reports: { box: string; report: Report }[], failed: string[] = [], now = Date.now()): UsageSummary {
  const today = reports.reduce((t, r) => (r.report.today > t ? r.report.today : t), "");
  const week = today ? days(today, 7).map((day) => ({ day, tokens: 0 })) : [];
  const at = new Map(week.map((d) => [d.day, d]));
  const agents = new Map<Agent, number>();
  let cost = 0;
  const windows: (UsageSummary["limits"][number] & { at: string })[] = [];
  for (const { report } of reports) {
    for (const [day, agent, , , , ...t] of report.daily) {
      const n = t[0] + t[1] + t[2] + t[3];
      const d = at.get(day);
      if (!d) continue;
      d.tokens += n;
      agents.set(agent, (agents.get(agent) ?? 0) + n);
    }
    const from = week[0]?.day ?? "";
    for (const s of report.sessions) if ((s.last ?? "") >= from && s.cost) cost += s.cost;
    // Each window the logins reported this week. One login read on several
    // boxes reports the same window, ending at the same moment give or take
    // a minute (Claude Code prints it to the minute, so two reads half a
    // minute apart can say 12:29 and 12:30): it is kept once, as most
    // recently read.
    for (const l of report.limits) {
      if (l.at.slice(0, 10) < from) continue;
      for (const w of [l.limits.primary, l.limits.secondary, ...(l.limits.extra ?? [])]) {
        if (!w?.resets_at) continue;
        const label = `${AGENT_NAME[l.agent]} · ${windowName(w)}`;
        const resets = w.resets_at * 1000;
        const i = windows.findIndex((x) => x.label === label && Math.abs(x.resets - resets) <= SAME_RESET_MS);
        const read = { label, used: w.used_percent / 100, resets, at: l.at };
        if (i < 0) windows.push(read);
        else if (windows[i].at < l.at) windows[i] = read;
      }
    }
  }
  return {
    today: at.get(today)?.tokens ?? 0,
    week,
    cost,
    byAgent: [...agents.entries()].map(([agent, tokens]) => ({ agent, tokens })).sort((a, b) => b.tokens - a.tokens),
    limits: windows.map(({ at: _, ...l }) => l).filter((l) => l.resets > now).sort((a, b) => b.used - a.used),
    failed,
  };
}

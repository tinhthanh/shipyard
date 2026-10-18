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

// One plan window of one login, as most recently read.
interface OpenWindow {
  agent: Agent;
  // The login: its email (and organisation, when it has one), or the
  // account name when it has none.
  login: string;
  // What it goes by: its email, or the account name.
  email: string;
  // The account folder it was read from ("default", "thanh-20x").
  account: string;
  // "Claude Code · 5h"
  label: string;
  // "5h", "week (Fable)"
  name: string;
  minutes: number;
  used: number;
  resets: number;
  at: string;
}

// openWindows is each window the logins reported since from (a day), still
// open at now. One login read on several boxes reports the same window,
// ending at the same moment give or take a minute (Claude Code prints it to
// the minute, so two reads half a minute apart can say 12:29 and 12:30): it
// is kept once, as most recently read. A login is its email and its
// organisation (one email can sign in to a team seat and a personal plan);
// every box has a "default".
function openWindows(reports: { report: Report }[], from: string, now: number): OpenWindow[] {
  const windows: OpenWindow[] = [];
  for (const { report } of reports) {
    for (const l of report.limits) {
      if (l.at.slice(0, 10) < from) continue;
      const email = l.email || l.account;
      const login = l.email && l.org ? `${l.email}#${l.org}` : email;
      for (const w of [l.limits.primary, l.limits.secondary, ...(l.limits.extra ?? [])]) {
        if (!w?.resets_at) continue;
        const name = windowName(w);
        const label = `${AGENT_NAME[l.agent]} · ${name}`;
        const resets = w.resets_at * 1000;
        const i = windows.findIndex((x) => x.label === label && x.login === login && Math.abs(x.resets - resets) <= SAME_RESET_MS);
        const read = { agent: l.agent, login, email, account: l.account, label, name, minutes: w.window_minutes, used: w.used_percent / 100, resets, at: l.at };
        if (i < 0) windows.push(read);
        else if (windows[i].at < l.at) windows[i] = read;
      }
    }
  }
  return windows.filter((w) => w.resets > now);
}

// loginNames is what each login goes by: the email's name, or the whole
// email when two logins of an agent share that name, and its account
// folder's name too when two share the email (two plans of one person).
// With only means just the agents that have more than one login.
function loginNames(windows: OpenWindow[], only = false): Map<string, string> {
  const who = new Map<string, string>();
  for (const agent of new Set(windows.map((w) => w.agent))) {
    const mine = windows.filter((w) => w.agent === agent);
    const logins = [...new Set(mine.map((w) => w.login))];
    if (only && logins.length < 2) continue;
    // Each login's email and account, as most recently read.
    const of = (l: string) => mine.filter((w) => w.login === l).reduce((a, b) => (b.at > a.at ? b : a));
    const emails = logins.map((l) => of(l).email);
    const names = emails.map((e) => e.split("@")[0]);
    logins.forEach((l, i) => {
      let name = names.filter((n) => n === names[i]).length > 1 ? emails[i] : names[i];
      if (emails.filter((e) => e === emails[i]).length > 1) name = `${names[i]} · ${of(l).account}`;
      who.set(`${agent}:${l}`, name);
    });
  }
  return who;
}

// The day a week of reports starts on: six days before the latest today.
const weekFrom = (reports: { report: Report }[]) => {
  const today = reports.reduce((t, r) => (r.report.today > t ? r.report.today : t), "");
  return today ? days(today, 7)[0] : "";
};

export interface LoginLimits {
  agent: Agent;
  login: string;
  // "ana", or the whole email when another login shares that name.
  name: string;
  // Shortest window first: the session, then the week, then a model's week.
  windows: { name: string; minutes: number; used: number; resets: number }[];
}

// limitsByLogin groups the open windows by login, for a ring each.
export function limitsByLogin(reports: { report: Report }[], now = Date.now()): LoginLimits[] {
  const live = openWindows(reports, weekFrom(reports), now);
  const who = loginNames(live);
  const out = new Map<string, LoginLimits>();
  for (const w of live) {
    const key = `${w.agent}:${w.login}`;
    const row = out.get(key) ?? { agent: w.agent, login: w.login, name: who.get(key) ?? w.login, windows: [] };
    row.windows.push({ name: w.name, minutes: w.minutes, used: w.used, resets: w.resets });
    out.set(key, row);
  }
  for (const row of out.values()) row.windows.sort((a, b) => a.minutes - b.minutes || a.name.localeCompare(b.name));
  return [...out.values()].sort((a, b) => a.agent.localeCompare(b.agent) || a.name.localeCompare(b.name));
}
// summarizeUsage adds up the boxes' reports for the widget.
export function summarizeUsage(reports: { box: string; report: Report }[], failed: string[] = [], now = Date.now()): UsageSummary {
  const today = reports.reduce((t, r) => (r.report.today > t ? r.report.today : t), "");
  const week = today ? days(today, 7).map((day) => ({ day, tokens: 0 })) : [];
  const at = new Map(week.map((d) => [d.day, d]));
  const agents = new Map<Agent, number>();
  let cost = 0;
  const from = week[0]?.day ?? "";
  for (const { report } of reports) {
    for (const [day, agent, , , , ...t] of report.daily) {
      const n = t[0] + t[1] + t[2] + t[3];
      const d = at.get(day);
      if (!d) continue;
      d.tokens += n;
      agents.set(agent, (agents.get(agent) ?? 0) + n);
    }
    for (const s of report.sessions) if ((s.last ?? "") >= from && s.cost) cost += s.cost;
  }
  const live = openWindows(reports, from, now);
  // With more than one login of an agent, each row says whose it is.
  const who = loginNames(live, true);
  return {
    today: at.get(today)?.tokens ?? 0,
    week,
    cost,
    byAgent: [...agents.entries()].map(([agent, tokens]) => ({ agent, tokens })).sort((a, b) => b.tokens - a.tokens),
    limits: live
      .map((w) => {
        const name = who.get(`${w.agent}:${w.login}`);
        return { label: name ? `${w.label} · ${name}` : w.label, used: w.used, resets: w.resets };
      })
      .sort((a, b) => b.used - a.used),
    failed,
  };
}

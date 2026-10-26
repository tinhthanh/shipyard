import type { LoginLimits } from "../../../plugins/usage/src/summary.ts";

// What the floating bar shows (app/bar, Model.swift): each login's open plan
// windows, and the agents that need you and those working, newest first.
// Kept free of the app's modules so it is tested on its own.

export interface BarAgent {
  key: string;
  box: string;
  session: string;
  agent: string;
  title: string;
  where: string;
}

export interface BarState {
  usage: boolean;
  logins: LoginLimits[];
  // Logins Claude Code lost on a box: "thanh-20x (thanh.huynh@acme.test)".
  signedOut: string[];
  waiting: BarAgent[];
  working: BarAgent[];
}

// The rows Home's agent widgets list (views/home/widgets/agents.tsx).
interface Row {
  key: string;
  box: string;
  session: { name: string };
  agent: string;
  title: string;
  where: string;
  state: string;
}

// The bar lists five of each; a few more say how many there are.
const MAX = 20;

export function barState(usage: boolean, logins: LoginLimits[], rows: Row[], signedOut: string[] = []): BarState {
  const pick = (state: string) =>
    rows
      .filter((r) => r.state === state)
      .slice(0, MAX)
      .map((r) => ({ key: r.key, box: r.box, session: r.session.name, agent: r.agent, title: r.title, where: r.where }));
  return { usage, logins: usage ? logins : [], signedOut: usage ? signedOut : [], waiting: pick("waiting"), working: pick("running") };
}

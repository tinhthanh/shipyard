// node --experimental-strip-types --test src/overlay/bar-state.test.ts (pnpm test)
import assert from "node:assert/strict";
import { test } from "node:test";

import { barState } from "./bar-state.ts";

const row = (state: string, name: string) => ({ key: `devl/${name}`, box: "devl", session: { name }, agent: "claude", title: `Task ${name}`, where: "shop · devl", state });
const login = { agent: "claude" as const, login: "ana@acme.test", name: "ana", windows: [{ name: "5h", minutes: 300, used: 0.3, resets: 1 }] };

test("the bar gets the agents that need you and those working, and nothing else", () => {
  const s = barState(true, [login], [row("waiting", "a"), row("running", "b"), row("finished", "c"), row("running", "d")]);
  assert.deepEqual(
    s.waiting.map((a) => a.session),
    ["a"],
  );
  assert.deepEqual(
    s.working.map((a) => a.session),
    ["b", "d"],
  );
  assert.deepEqual(s.waiting[0], { key: "devl/a", box: "devl", session: "a", agent: "claude", title: "Task a", where: "shop · devl" });
  assert.equal(s.logins.length, 1);
});

test("without the Usage plugin the bar gets no plan limits", () => {
  assert.deepEqual(barState(false, [login], []).logins, []);
});

test("a long list is cut short", () => {
  const many = Array.from({ length: 50 }, (_, i) => row("running", `s${i}`));
  assert.equal(barState(true, [], many).working.length, 20);
});

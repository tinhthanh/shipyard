// node --experimental-strip-types --test src/lib/plugin-boxes.test.ts (pnpm test)
import assert from "node:assert/strict";
import { test } from "node:test";

import { boxRowView } from "./plugin-boxes.ts";

test("a box without the plugin can take it", () => {
  assert.deepEqual(boxRowView({ box: "devl", state: "off" }), { on: false, canToggle: true, note: "Not on this box", tone: "muted" });
});

test("an older version on the box offers the update", () => {
  const v = boxRowView({ box: "devl", state: "outdated", version: "1.0.0" });
  assert.equal(v.action, "update");
  assert.equal(v.on, true);
  assert.match(v.note, /1\.0\.0/);
});

test("on the box but not set up offers the setup, with the plugin's own words", () => {
  const v = boxRowView({ box: "devl", state: "on", setup: { ready: false, known: true, label: "Allow Accessibility" } });
  assert.equal(v.action, "setup");
  assert.equal(v.tone, "warning");
  assert.match(v.note, /Allow Accessibility/);
});

test("set up on the box needs nothing more", () => {
  const v = boxRowView({ box: "devl", state: "on", setup: { ready: true, known: true } });
  assert.equal(v.action, undefined);
  assert.equal(v.tone, "success");
});

test("a box it can't run on, or that can't take plugins, can't be switched", () => {
  assert.equal(boxRowView({ box: "ci", state: "unsupported", error: "built for darwin-arm64; ci is linux-amd64" }).canToggle, false);
  assert.equal(boxRowView({ box: "old", state: "old_berthd" }).canToggle, false);
  assert.equal(boxRowView({ box: "away", state: "error", error: "away" }).tone, "error");
});

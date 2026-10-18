// node --experimental-strip-types --test src/lib/plugin-boxes.test.ts (pnpm test)
import assert from "node:assert/strict";
import { test } from "node:test";

import { BOX_POLL_IDLE_MS, BOX_POLL_SETUP_MS, boxPollMs, boxRowView, outdatedBoxes } from "./plugin-boxes.ts";

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

test("the rows look often only for a while after a setup was started", () => {
  assert.equal(boxPollMs(undefined, 1_000_000), BOX_POLL_IDLE_MS);
  assert.equal(boxPollMs(1_000_000, 1_000_000 + 10_000), BOX_POLL_SETUP_MS);
  assert.equal(boxPollMs(1_000_000, 1_000_000 + 121_000), BOX_POLL_IDLE_MS);
});

test("a box's setup says what is left in the plugin's words, or that its screen is locked", () => {
  const setup = { label: "Load the extension", ready: false, known: true };
  assert.equal(boxRowView({ box: "devl", state: "on", setup }).note, "Needs setup on the box: Load the extension");
  assert.equal(boxRowView({ box: "devl", state: "on", setup: { ...setup, problem: "Google Chrome isn't installed" } }).note, "Google Chrome isn't installed");
  const locked = boxRowView({ box: "devl", state: "on", setup: { ...setup, problem: "Google Chrome isn't installed", locked: true } });
  assert.match(locked.note, /screen is locked/);
  assert.equal(locked.action, "setup");
});

test("update all goes to the boxes with another version", () => {
  assert.deepEqual(
    outdatedBoxes([
      { box: "a", state: "outdated" },
      { box: "b", state: "on" },
      { box: "c", state: "outdated" },
      { box: "d", state: "unsupported" },
    ]),
    ["a", "c"],
  );
});

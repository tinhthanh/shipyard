// Runs usage.py's own tests (usage_test.py) in the app's test run, so the
// box-side script is checked where CI checks everything else.
import { spawnSync } from "node:child_process";
import { dirname } from "node:path";
import test from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const python = spawnSync("python3", ["--version"]).status === 0;

test("usage.py reads Claude Code's /usage", { skip: !python && "no python3" }, () => {
  // A hung run fails the test rather than holding up the whole suite.
  const r = spawnSync("python3", ["-m", "unittest", "-q", "usage_test.py"], { cwd: here, encoding: "utf8", timeout: 120_000 });
  assert.equal(r.status, 0, r.stderr || r.stdout);
});

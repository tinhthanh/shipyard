// node --experimental-strip-types --test src/lib/fork-updates.test.ts (pnpm test)
import assert from "node:assert/strict";
import { test } from "node:test";

import { noAppRelease } from "./fork-updates.ts";

test("a release with no app to update to is not an error", () => {
  assert.equal(noAppRelease("Could not fetch a valid release JSON from the remote"), true);
  assert.equal(noAppRelease("download of latest.json failed: 404 Not Found"), true);
  assert.equal(noAppRelease("None of the fallback platforms `[\"darwin-aarch64-app\", \"darwin-aarch64\"]` were found in the response `platforms` object"), true);
  assert.equal(noAppRelease("the platform `linux-x86_64` was not found in the response `platforms` object"), true);
});

test("other failures still show", () => {
  assert.equal(noAppRelease("error sending request: dns error"), false);
  assert.equal(noAppRelease("signature verification failed"), false);
});

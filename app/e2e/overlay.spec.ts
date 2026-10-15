import { expect, mockOnly, test } from "./fixtures";

// The floating bar's page (src/overlay, ?window=overlay): what its window
// shows, on the mock fleet. The window itself (always on top, over every
// app) is the app's, and isn't here.

const open = async (page: import("@playwright/test").Page, place: "edge" | "notch", usage = true) => {
  await page.addInitScript(
    (prefs) => {
      if (sessionStorage.getItem("e2e.seeded")) return;
      sessionStorage.setItem("e2e.seeded", "1");
      localStorage.setItem("berth.prefs", JSON.stringify(prefs));
    },
    { whatsNewSeen: "999.0.0", enabledPlugins: usage ? ["usage"] : [] },
  );
  await page.goto(`/?mock=1&window=overlay&place=${place}`);
};

test("the bar shows a ring for each login and the agents that need you or are working", async ({ app }) => {
  mockOnly();
  const page = app.page;
  await open(page, "edge");
  const bar = page.getByTestId("overlay-bar");
  await expect(page.getByTestId("overlay")).toHaveAttribute("data-place", "edge");
  // Two Claude logins whose emails share a name show whole; Codex's one is its name.
  await expect(bar.getByRole("button", { name: /^you@example\.com: 5h 31%/ })).toBeVisible();
  await expect(bar.getByRole("button", { name: /^you@work\.example: 5h 12%/ })).toBeVisible();
  await expect(bar.getByRole("button", { name: /^you: 5h 34%/ })).toBeVisible();
  await expect(page.getByTestId("overlay-waiting")).toHaveText("2");
  await expect(page.getByTestId("overlay-working")).toHaveText("2");
});

test("hovering the bar opens a card with each window's reset and the agents, a click from opening", async ({ app }) => {
  mockOnly();
  const page = app.page;
  await open(page, "edge");
  await page.getByTestId("overlay-bar").hover();
  const card = page.getByTestId("overlay-card");
  await expect(card).toBeVisible();
  await expect(card.getByText("week (Fable)")).toBeVisible();
  await expect(card.getByText("Need you · 2")).toBeVisible();
  await expect(card.getByText("Working · 2")).toBeVisible();
  // In the app the click opens the session in the main window; here the
  // page says what it would open.
  const opened = page.evaluate(() => new Promise((r) => window.addEventListener("berth://overlay-open", (e) => r((e as CustomEvent).detail), { once: true })));
  await card.getByRole("button", { name: /Fix checkout webhook retries/ }).click();
  expect(await opened).toEqual({ box: "devl", session: "checkout-fix-claude" });
});

test("at the notch the two sides are the same width, so the gap stays over the notch", async ({ app }) => {
  mockOnly();
  const page = app.page;
  await open(page, "notch");
  const sides = page.getByTestId("overlay-bar").locator(":scope > div");
  await expect(sides).toHaveCount(3);
  const widths = await sides.evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().width)));
  expect(widths[0]).toBe(widths[2]);
});

test("without the Usage plugin the card says how to get plan limits", async ({ app }) => {
  mockOnly();
  const page = app.page;
  await open(page, "edge", false);
  await page.getByTestId("overlay-bar").hover();
  await expect(page.getByTestId("overlay-card").getByText("Turn on the Usage & accounts plugin to see plan limits here.")).toBeVisible();
});

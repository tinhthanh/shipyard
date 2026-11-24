import { expect, mockOnly, test } from "./fixtures";

// Pinned projects (the fork's, components/sidebar/project-pins.tsx): once
// one is pinned, the rail shows only the pinned and the one you are in; the
// sheet's heading opens every project, to switch to or to pin. On the mock
// fixtures: shop, evals and notes.

const rail = { sidebarLayout: "rail" };

test("pinning keeps only the pinned projects on the rail, and the heading switches to any", async ({ app }) => {
  mockOnly();
  await app.open({ prefs: rail });
  const page = app.page;
  const tiles = page.getByTestId("rail-project");
  // Nothing pinned yet: every project, as before.
  await expect(tiles).toHaveCount(3);
  await page.locator("[data-testid=rail-project][data-project=shop]").click();
  await expect(page.locator("[data-testid=rail-project][data-project=shop]")).toHaveAttribute("aria-current", "true");

  // Pin evals from the switcher: the rail keeps it and shop (you are in shop).
  await page.getByTestId("project-switcher").click();
  const popup = page.getByTestId("project-switcher-popup");
  await expect(popup.getByTestId("switcher-project")).toHaveCount(3);
  await popup.getByRole("button", { name: "Pin evals to the rail" }).click();
  await expect(tiles).toHaveCount(2);
  await expect(page.locator("[data-testid=rail-project][data-project=notes]")).toHaveCount(0);
  expect(await app.stored("berth.projectPins")).toEqual([expect.any(String)]);

  // Switch to notes from the heading: it shows while you are in it.
  await popup.getByRole("textbox", { name: "Find a project" }).fill("not");
  await expect(popup.getByTestId("switcher-project")).toHaveCount(1);
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog", { name: "New task" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator("[data-testid=rail-project][data-project=notes]")).toHaveAttribute("aria-current", "true");
  await expect(page.locator("[data-testid=rail-project][data-project=shop]")).toHaveCount(0);
  await expect(tiles).toHaveCount(2);

  // Show all: every project on the rail again.
  await page.getByTestId("project-switcher").click();
  await page.getByTestId("switcher-show-all").click();
  await expect(tiles).toHaveCount(3);
});

test("a project's menu pins it to the rail and unpins it", async ({ app }) => {
  mockOnly();
  await app.open({ prefs: rail });
  const page = app.page;
  await expect(page.getByTestId("rail-project")).toHaveCount(3);
  await page.locator("[data-testid=rail-project][data-project=shop]").click();
  await expect(page.locator("[data-testid=rail-project][data-project=shop]")).toHaveAttribute("aria-current", "true");
  await page.locator("[data-testid=rail-project][data-project=evals]").click({ button: "right" });
  await page.getByRole("menuitem", { name: "Pin to the rail" }).click();
  // evals pinned, and shop, which you are in.
  await expect(page.getByTestId("rail-project")).toHaveCount(2);
  await page.locator("[data-testid=rail-project][data-project=evals]").click({ button: "right" });
  await page.getByRole("menuitem", { name: "Unpin from the rail" }).click();
  // Nothing pinned now: only the project you are in.
  await expect(page.getByTestId("rail-project")).toHaveCount(1);
  await expect(page.locator("[data-testid=rail-project][data-project=shop]")).toHaveCount(1);
});

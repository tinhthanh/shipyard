import { expect, mockOnly, test } from "./fixtures";
import { closeSheet, listedCard as listed, openPlugin, pluginItem as installed, pluginSheet } from "./plugin-store";

// Settings → Plugins → Discover: plugins that aren't built into the app,
// listed by the laptop agent and installed into ~/.berth/plugins. The mock
// (src/lib/mock-marketplace.ts) lists three, one per state a card has:
// Standup notes (not installed), Deploy board (installed from the listing,
// a version behind) and hello-ports (a plugin of your own with that id).
// A plugin's switch, Remove… and the rest are in the sheet its item opens
// (e2e/plugin-store.ts).
//
// Plugins load after the page does, and every install reloads them, so each
// step waits on the item, sheet or dialog it expects, never on time.

test("Discover lists each plugin with what can be done about it, and says where the listing is", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  await expect(section.getByText(/Listed at\s*berthd\.app/)).toBeVisible();
  const standup = listed(section, "acme-standup");
  await expect(standup).toContainText("Standup notes");
  await expect(standup).toContainText("1.0.0");
  await expect(standup).toContainText("What each agent finished since yesterday");
  await expect(standup).toContainText("by Acme");
  await expect(standup.getByRole("button", { name: "Install", exact: true })).toBeVisible();

  const deploys = listed(section, "acme-deploys");
  await expect(deploys.getByRole("button", { name: "Update", exact: true })).toBeVisible();
  await expect(deploys).toContainText("You have 1.1.0.");
  // A newer listing shows on top too, with one button for all.
  await expect(section.getByTestId("plugin-updates")).toContainText("Deploy board 1.2.0 is available");

  // A folder of your own with a listed id is never installed over unasked.
  const hello = listed(section, "hello-ports");
  await expect(hello.getByRole("button", { name: "Replace…" })).toBeVisible();
  await expect(hello).toContainText("is your own");
});

test("search narrows every section, and the filter shows one", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  await section.getByTestId("plugin-search").fill("standup");
  await expect(listed(section, "acme-standup")).toBeVisible();
  await expect(listed(section, "acme-deploys")).toHaveCount(0);
  await expect(installed(section, "hello-ports")).toHaveCount(0);
  await section.getByTestId("plugin-search").fill("");

  await section.getByRole("button", { name: "Built in", exact: true }).click();
  await expect(section.getByRole("heading", { name: "Built in" })).toBeVisible();
  await expect(installed(section, "notes")).toBeVisible();
  await expect(section.getByTestId("market-plugin")).toHaveCount(0);
});

test("an installed plugin shows under your plugins, off, until the consent dialog turns it on", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  await expect(installed(section, "acme-standup")).toHaveCount(0);
  await listed(section, "acme-standup").getByRole("button", { name: "Install", exact: true }).click();

  const item = installed(section, "acme-standup");
  await expect(item).toContainText("Standup notes");
  await expect(item).toContainText("from the listing");
  await expect(item).toContainText("Off");
  await expect(listed(section, "acme-standup")).toContainText("Installed");
  await expect(listed(section, "acme-standup").getByRole("button", { name: "Install", exact: true })).toHaveCount(0);
  // Installing asked nothing and turned nothing on: the listing is read
  // again only after the plugins have reloaded, which is when a consent
  // dialog would have opened. (A toast is a dialog too, hence the name.)
  const consent = app.page.getByRole("dialog", { name: "Allow Standup notes?" });
  await expect(consent).toHaveCount(0);
  await expect(app.page.getByText("It's off until you turn it on.")).toBeVisible();

  // Turning it on is the usual consent: what it can do, and its hash.
  const row = await openPlugin(app.page, section, "acme-standup");
  const toggle = row.getByRole("switch");
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await toggle.click();
  await expect(consent).toContainText("This plugin isn't part of Shipyard.");
  await expect(consent).toContainText("~/.berth/plugins/acme-standup");
  await expect(consent).toContainText("sha256:");
  await consent.getByRole("button", { name: "Allow and turn on" }).click();
  await expect(consent).toHaveCount(0);
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  // It really loaded: the row lists what it added.
  await expect(row).toContainText("1 command");
  await closeSheet(app.page);
  await expect(item).toContainText("On");
});

test("declining the consent dialog leaves an installed plugin off", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  await listed(section, "acme-standup").getByRole("button", { name: "Install", exact: true }).click();
  const row = await openPlugin(app.page, section, "acme-standup");
  const toggle = row.getByRole("switch");
  await toggle.click();
  const consent = app.page.getByRole("dialog", { name: "Allow Standup notes?" });
  await consent.getByRole("button", { name: "Not now" }).click();
  await expect(consent).toHaveCount(0);
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await expect(row).not.toContainText("1 command");
});

test("an update replaces a plugin that was on, and asks before the new code runs", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  // Deploy board 1.1.0 is installed; allow it as it is.
  const item = installed(section, "acme-deploys");
  await expect(item).toContainText("1.1.0");
  const row = await openPlugin(app.page, section, "acme-deploys");
  await row.getByRole("switch").click();
  await app.page.getByRole("dialog", { name: "Allow Deploy board?" }).getByRole("button", { name: "Allow and turn on" }).click();
  await expect(row.getByRole("switch")).toHaveAttribute("aria-checked", "true");
  await closeSheet(app.page);

  await listed(section, "acme-deploys").getByRole("button", { name: "Update", exact: true }).click();
  // The new version is not what was allowed: it is off, and Shipyard asks.
  const changed = app.page.getByRole("dialog", { name: "Deploy board has changed" });
  await expect(changed).toContainText("acme-deploys 1.2.0");
  await expect(item).toContainText("1.2.0");
  await expect(listed(section, "acme-deploys")).toContainText("Installed");

  await changed.getByRole("button", { name: "Allow and turn on" }).click();
  await expect(changed).toHaveCount(0);
  await expect(item).toContainText("On");
  await expect(await openPlugin(app.page, section, "acme-deploys")).toContainText("1 command");
});

test("a plugin of your own is replaced only after confirming", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  const hello = listed(section, "hello-ports");
  await hello.getByRole("button", { name: "Replace…" }).click();
  const confirm = app.page.getByRole("alertdialog", { name: "Replace your Hello ports (example)?" });
  await expect(confirm).toContainText("~/.berth/plugins/hello-ports");
  await confirm.getByRole("button", { name: "Cancel" }).click();
  await expect(confirm).toHaveCount(0);
  await expect(installed(section, "hello-ports")).toContainText("your own");

  await hello.getByRole("button", { name: "Replace…" }).click();
  await confirm.getByRole("button", { name: "Replace", exact: true }).click();
  await expect(installed(section, "hello-ports")).toContainText("from the listing");
  await expect(hello).toContainText("Installed");
  await expect(hello.getByRole("button", { name: "Replace…" })).toHaveCount(0);
  await expect((await openPlugin(app.page, section, "hello-ports")).getByRole("switch")).toHaveAttribute("aria-checked", "false");
});

test("a plugin installed from the listing can be removed, and installed again", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  // Only the listing's plugins offer it: a folder of your own is yours.
  await expect((await openPlugin(app.page, section, "hello-ports")).getByRole("button", { name: "Remove…" })).toHaveCount(0);
  await closeSheet(app.page);
  const row = await openPlugin(app.page, section, "acme-deploys");
  await row.getByRole("button", { name: "Remove…" }).click();
  const confirm = app.page.getByRole("alertdialog", { name: "Remove Deploy board?" });
  await confirm.getByRole("button", { name: "Remove", exact: true }).click();
  // Its sheet closes with it.
  await expect(pluginSheet(app.page)).toHaveCount(0);
  await expect(installed(section, "acme-deploys")).toHaveCount(0);
  await expect(listed(section, "acme-deploys").getByRole("button", { name: "Install", exact: true })).toBeVisible();
});

test("a listed plugin's card opens what installing it does, before anything runs", async ({ app }) => {
  mockOnly();
  await app.open();
  const section = await app.openSettings("plugins");

  await listed(section, "acme-standup").getByRole("button", { name: "Standup notes: details" }).click();
  const sheet = pluginSheet(app.page);
  await expect(sheet).toContainText("What each agent finished since yesterday");
  await expect(sheet).toContainText("~/.berth/plugins/acme-standup");
  await sheet.getByRole("button", { name: "Install", exact: true }).click();
  await expect(sheet).toHaveCount(0);
  await expect(installed(section, "acme-standup")).toContainText("Off");
});

test("when the listing can't be reached, Discover says so and reads it again on request", async ({ app }) => {
  mockOnly();
  await app.open({ params: { market: "flaky" } });
  const section = await app.openSettings("plugins");

  const error = section.getByTestId("market-error");
  await expect(error).toContainText("Shipyard couldn't reach the plugin listing.");
  // What is installed doesn't need the listing.
  await expect(installed(section, "acme-deploys")).toBeVisible();
  await expect(section.getByTestId("market-plugin")).toHaveCount(0);

  await error.getByRole("button", { name: "Try again" }).click();
  await expect(listed(section, "acme-standup").getByRole("button", { name: "Install", exact: true })).toBeVisible();
  await expect(error).toHaveCount(0);
});

test("an install that fails its checksum says why and installs nothing", async ({ app }) => {
  mockOnly();
  await app.open({ params: { market: "mismatch" } });
  const section = await app.openSettings("plugins");

  const install = listed(section, "acme-standup").getByRole("button", { name: "Install", exact: true });
  await install.click();
  await expect(app.page.getByText("Couldn't install Standup notes")).toBeVisible();
  await expect(app.page.getByText("What was downloaded isn't what the listing describes, so nothing was installed.")).toBeVisible();
  await expect(installed(section, "acme-standup")).toHaveCount(0);
  // The card is as it was, ready to try again.
  await expect(install).toBeEnabled();
});

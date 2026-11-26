import { expect, type Locator, type Page } from "@playwright/test";

// Settings → Plugins as the fork lays it out (src/views/settings/plugin-store/):
// your plugins as one-line items, the listing as cards, and each plugin's
// controls in a sheet its item opens. The sheet holds upstream's own row
// (data-testid=plugin-row), switch, Remove… and all.

export const pluginItem = (section: Locator, id: string) => section.locator(`[data-testid=plugin-item][data-plugin="${id}"]`);
export const listedCard = (section: Locator, id: string) => section.locator(`[data-testid=market-plugin][data-plugin="${id}"]`);
export const pluginSheet = (page: Page) => page.getByTestId("plugin-sheet");

// openPlugin opens a plugin's sheet from its item, unless it is open
// already, and returns the row inside it.
export async function openPlugin(page: Page, section: Locator, id: string): Promise<Locator> {
  const row = pluginSheet(page).locator(`[data-testid=plugin-row][data-plugin="${id}"]`);
  if (!(await row.isVisible())) await pluginItem(section, id).getByRole("button", { name: /: details$/ }).click();
  await expect(row).toBeVisible();
  return row;
}

export async function closeSheet(page: Page) {
  await page.keyboard.press("Escape");
  await expect(pluginSheet(page)).toHaveCount(0);
}

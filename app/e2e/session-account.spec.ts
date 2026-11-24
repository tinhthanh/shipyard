import { expect, mockOnly, test } from "./fixtures";

// Which account a Claude session signs in with, and moving its
// conversation to another (the fork's: components/conversation/
// account-switch.tsx, berthd's sessionaccount.go). On the mock fixtures,
// checkout-fix's Claude signs in as work; devl has a default login too, and
// one signed out.

test("a Claude chat says its account, and moves to another one", async ({ app }) => {
  mockOnly("the mock's sessions carry their accounts");
  await app.open({ params: { view: "conversation" } });
  await app.openWorktree("devl/checkout-fix");
  await expect(app.chat).toBeVisible();
  const page = app.page;
  const chip = page.getByTestId("account-chip");
  await expect(chip).toHaveText("work");

  await chip.click();
  const dialog = page.getByTestId("account-switch");
  await expect(dialog).toBeVisible();
  // The other accounts: default to move to, lapsed signed out and not offered.
  const options = dialog.getByTestId("account-switch-option");
  await expect(options.filter({ hasText: "default" })).toBeEnabled();
  await expect(options.filter({ hasText: "lapsed" })).toBeDisabled();
  await expect(options.filter({ hasText: "work" })).toHaveCount(0);
  await expect(dialog).toContainText("reads the whole conversation again");

  await dialog.getByTestId("account-switch-go").click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("checkout-fix-claude is on default now")).toBeVisible();
  await expect(chip).toHaveText("default");
});
